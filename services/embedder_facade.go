package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"gorm.io/gorm"
)

// EmbedderInput is how a datasource write names its embedder. EmbedderID,
// when set, wins (0 unlinks). Otherwise the legacy inline fields (the
// datasource API's embed_vendor/url/api_key/model) describe the embedding
// configuration and are resolved to an embedder: see resolveDatasourceEmbedder.
type EmbedderInput struct {
	EmbedderID *uint
	Vendor     string
	URL        string
	APIKey     string
	Model      string

	// VectorConn and VectorAPIKey are the datasource's vector store
	// connection string and key. The Vertex embedder used to read its
	// project:location and key from them, and clients (the portal's
	// submission form among them) still send Vertex settings that way: a
	// Vertex configuration without its own URL takes them.
	VectorConn   string
	VectorAPIKey string

	// Namespace is the datasource's namespace: an embedder linked to an LLM
	// scoped to another namespace is refused (models.UsableInNamespace).
	Namespace string
}

// resolveDatasourceEmbedder decides which embedder a datasource write links
// to, creating one when needed, and checks the embedder may see the
// datasource's data (its privacy score is at least the datasource's).
//
// An explicit EmbedderID links that embedder. Otherwise the legacy fields are
// merged onto the current embedder's configuration with the datasource API's
// long-standing rules: an empty vendor, URL or model keeps the current value;
// a key of "[redacted]" keeps it and "" clears it. The merged configuration
// then:
//   - is the current embedder when nothing changed (no write);
//   - for a linked embedder whose connection is unchanged, is the embedder
//     linked to the same LLM with the new model (found or created);
//   - otherwise is a standalone embedder with exactly that configuration
//     (found or created) whose privacy score covers the datasource.
//
// A shared embedder is never modified through a datasource: a change always
// moves this datasource to another embedder, leaving the others as they were.
func (s *Service) resolveDatasourceEmbedder(tx *gorm.DB, current *models.Embedder, in EmbedderInput, datasourceName string, privacyScore int, userID uint) (*models.Embedder, error) {
	e, err := s.pickDatasourceEmbedder(tx, current, in, datasourceName, privacyScore, userID)
	if err != nil || e == nil {
		return e, err
	}
	if err := e.UsableInNamespace(in.Namespace); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedderInvalid, err)
	}
	return e, nil
}

// pickDatasourceEmbedder is resolveDatasourceEmbedder without the namespace
// check.
func (s *Service) pickDatasourceEmbedder(tx *gorm.DB, current *models.Embedder, in EmbedderInput, datasourceName string, privacyScore int, userID uint) (*models.Embedder, error) {
	if in.EmbedderID != nil {
		if *in.EmbedderID == 0 {
			return nil, nil
		}
		var e models.Embedder
		if err := e.Get(tx, *in.EmbedderID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: embedder %d does not exist", ErrEmbedderInvalid, *in.EmbedderID)
			}
			return nil, err
		}
		if score := e.EffectivePrivacyScore(); score < privacyScore {
			return nil, &EmbedderPrivacyError{EmbedderScore: score, Required: privacyScore, Datasource: datasourceName}
		}
		return &e, nil
	}

	var cur models.EmbedderSpec
	if current != nil {
		if spec, err := current.Spec(false); err == nil {
			cur = *spec
		}
	}
	want := cur
	if in.Vendor != "" {
		want.Vendor = models.Vendor(in.Vendor)
	}
	if in.URL != "" {
		want.Endpoint = in.URL
	}
	if in.APIKey != RedactedEmbedderKey {
		want.APIKey = in.APIKey
	}
	if in.Model != "" {
		want.Model = in.Model
	}
	if want.Vendor == models.VERTEX && want.Endpoint == "" {
		want.Endpoint = in.VectorConn
		if want.APIKey == "" {
			want.APIKey = in.VectorAPIKey
		}
	}

	if want.Vendor == "" {
		// No embedding configuration at all: keep what is there (a
		// datasource may exist before its embedder is chosen).
		return current, nil
	}

	sameConnection := current != nil && want.Vendor == cur.Vendor && want.Endpoint == cur.Endpoint && want.APIKey == cur.APIKey
	if sameConnection && want.Model == cur.Model && cur.PrivacyScore >= privacyScore {
		return current, nil
	}
	if sameConnection && current.IsLinked() {
		e, err := s.findOrCreateLinkedEmbedder(tx, *current.LLMID, want.Model, userID)
		if err != nil {
			return nil, err
		}
		if score := e.EffectivePrivacyScore(); score < privacyScore {
			return nil, &EmbedderPrivacyError{EmbedderScore: score, Required: privacyScore, Datasource: datasourceName}
		}
		return e, nil
	}
	return s.findOrCreateStandaloneEmbedder(tx, want, privacyScore, userID)
}

// findOrCreateStandaloneEmbedder returns a standalone embedder with exactly
// this configuration and a privacy score of at least minPrivacy, creating one
// (named "<vendor> · <model>") when there is none.
//
// Concurrent writes with the same configuration create one embedder: the
// find-or-create holds an advisory lock on it (models.LockEmbedderConfig).
func (s *Service) findOrCreateStandaloneEmbedder(db *gorm.DB, spec models.EmbedderSpec, minPrivacy int, userID uint) (*models.Embedder, error) {
	var e models.Embedder
	created := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := models.LockEmbedderConfig(tx, "standalone", string(spec.Vendor), spec.Endpoint, spec.APIKey, spec.Model); err != nil {
			return err
		}
		ferr := tx.Where("llm_id IS NULL AND vendor = ? AND endpoint = ? AND api_key = ? AND model = ? AND privacy_score >= ?",
			spec.Vendor, spec.Endpoint, spec.APIKey, spec.Model, minPrivacy).
			Order("id").First(&e).Error
		if ferr == nil {
			return nil
		}
		if !errors.Is(ferr, gorm.ErrRecordNotFound) {
			return ferr
		}
		e = models.Embedder{
			Vendor:       spec.Vendor,
			Endpoint:     spec.Endpoint,
			APIKey:       spec.APIKey,
			ModelName:    spec.Model,
			PrivacyScore: minPrivacy,
			UserID:       userID,
		}
		// Not validated: the legacy datasource fields never were (a
		// datasource could name a vendor that cannot embed, or no model
		// yet), and a write through them keeps working as it did. The
		// embedder shows up in the Embedders list, where it can be fixed;
		// the log says why it cannot embed yet.
		if err := models.CreateWithDefaultName(tx, &e, string(spec.Vendor), spec.Model); err != nil {
			return err
		}
		created = true
		if err := s.validateEmbedder(tx, &e); err != nil {
			logger.Warn(fmt.Sprintf("embedder %q (id %d) was created from datasource embed_* fields but cannot embed yet: %v", e.Name, e.ID, err))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if created {
		s.emitEmbedder(&e, "created", userID)
	}
	return &e, nil
}

// FindOrCreateLinkedEmbedder returns the embedder linked to the LLM with this
// model, creating one when there is none.
func (s *Service) FindOrCreateLinkedEmbedder(llmID uint, model string, userID uint) (*models.Embedder, error) {
	return s.findOrCreateLinkedEmbedder(s.DB, llmID, model, userID)
}

func (s *Service) findOrCreateLinkedEmbedder(tx *gorm.DB, llmID uint, model string, userID uint) (*models.Embedder, error) {
	var llm models.LLM
	if err := tx.First(&llm, llmID).Error; err == nil && !switches.SupportsEmbeddings(llm.Vendor) {
		return nil, fmt.Errorf("%w: vendor %q does not provide embeddings", ErrEmbedderInvalid, llm.Vendor)
	}
	e, created, err := models.FindOrCreateLinkedEmbedder(tx, llmID, model, userID)
	if err != nil {
		return nil, err
	}
	if created {
		s.emitEmbedder(e, "created", userID)
	}
	return e, nil
}

// applyHookEmbedEdits re-resolves the embedder of a datasource a plugin hook
// returned modified. Hooks see the legacy embed_* keys; when the plugin
// changed them, the datasource moves to a matching embedder (as a legacy API
// write would). A hook that only changed embedder_id links that embedder,
// and one that returned neither (a plugin building the object itself) keeps
// the embedder the datasource had: silence is not a request to unlink.
func (s *Service) applyHookEmbedEdits(tx *gorm.DB, original *models.Embedder, modified *models.Datasource, userID uint) error {
	var in EmbedderInput
	if le, ok := modified.LegacyEmbedInput(); ok {
		in = EmbedderInput{Vendor: le.Vendor, URL: le.URL, APIKey: le.APIKey, Model: le.Model}
		if in.APIKey == models.RedactedLegacyKey {
			in.APIKey = RedactedEmbedderKey
		}
		if originalID(original) != idOf(modified.EmbedderID) {
			in = EmbedderInput{EmbedderID: embedderIDOrZero(modified.EmbedderID)}
		}
	} else if modified.EmbedderID != nil {
		in = EmbedderInput{EmbedderID: embedderIDOrZero(modified.EmbedderID)}
	} else {
		keep := originalID(original)
		in = EmbedderInput{EmbedderID: &keep}
	}
	in.VectorConn, in.VectorAPIKey, in.Namespace = modified.DBConnString, modified.DBConnAPIKey, modified.Namespace
	e, err := s.resolveDatasourceEmbedder(tx, original, in, modified.Name, modified.PrivacyScore, userID)
	if err != nil {
		return err
	}
	modified.SetEmbedder(e)
	return nil
}

func originalID(e *models.Embedder) uint {
	if e == nil {
		return 0
	}
	return e.ID
}

func idOf(id *uint) uint {
	if id == nil {
		return 0
	}
	return *id
}
