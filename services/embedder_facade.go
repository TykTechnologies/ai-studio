package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
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
}

// LegacyEmbedFields is a datasource's embedder flattened into the fields the
// datasource API, gRPC service, object hooks and edge snapshot carry.
type LegacyEmbedFields struct {
	Vendor models.Vendor
	URL    string
	APIKey string
	Model  string
}

// FlattenDatasourceEmbedding returns the datasource's embedder as the legacy
// inline fields. With resolveSecrets the key and endpoint are resolved (for
// runtime and edge use); without, references are returned as stored (for API
// responses). A datasource without a (resolvable) embedder flattens to empty
// fields. The datasource must have Embedder (and its LLM) preloaded.
func FlattenDatasourceEmbedding(ds *models.Datasource, resolveSecrets bool) LegacyEmbedFields {
	if ds == nil {
		return LegacyEmbedFields{}
	}
	f := ds.EmbedFields(resolveSecrets)
	return LegacyEmbedFields{Vendor: models.Vendor(f.Vendor), URL: f.URL, APIKey: f.APIKey, Model: f.Model}
}

// DatasourceEmbedderSpec resolves the datasource's embedder for runtime use.
// The datasource must have Embedder (and its LLM) preloaded.
func DatasourceEmbedderSpec(ds *models.Datasource) (*models.EmbedderSpec, error) {
	if ds == nil || ds.Embedder == nil {
		return nil, errors.New("datasource has no embedder")
	}
	return ds.Embedder.Spec(true)
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
func (s *Service) findOrCreateStandaloneEmbedder(tx *gorm.DB, spec models.EmbedderSpec, minPrivacy int, userID uint) (*models.Embedder, error) {
	var e models.Embedder
	err := tx.Where("llm_id IS NULL AND vendor = ? AND endpoint = ? AND api_key = ? AND model = ? AND privacy_score >= ?",
		spec.Vendor, spec.Endpoint, spec.APIKey, spec.Model, minPrivacy).
		Order("id").First(&e).Error
	if err == nil {
		return &e, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	name, err := models.UniqueEmbedderName(tx, string(spec.Vendor), spec.Model)
	if err != nil {
		return nil, err
	}
	e = models.Embedder{
		Name:         name,
		Vendor:       spec.Vendor,
		Endpoint:     spec.Endpoint,
		APIKey:       spec.APIKey,
		ModelName:    spec.Model,
		PrivacyScore: minPrivacy,
		UserID:       userID,
	}
	// Not validated: the legacy datasource fields never were (a datasource
	// could name a vendor that cannot embed, or no model yet), and a write
	// through them keeps working as it did. The embedder shows up in the
	// Embedders list, where it can be fixed.
	if err := e.Create(tx); err != nil {
		return nil, err
	}
	s.emitEmbedder(&e, "created", userID)
	return &e, nil
}

// FindOrCreateLinkedEmbedder returns the embedder linked to the LLM with this
// model, creating one when there is none.
func (s *Service) FindOrCreateLinkedEmbedder(llmID uint, model string, userID uint) (*models.Embedder, error) {
	return s.findOrCreateLinkedEmbedder(s.DB, llmID, model, userID)
}

func (s *Service) findOrCreateLinkedEmbedder(tx *gorm.DB, llmID uint, model string, userID uint) (*models.Embedder, error) {
	var e models.Embedder
	err := tx.Preload("LLM").Where("llm_id = ? AND model = ?", llmID, model).Order("id").First(&e).Error
	if err == nil {
		return &e, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var llm models.LLM
	if err := tx.First(&llm, llmID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: LLM %d does not exist", ErrEmbedderInvalid, llmID)
		}
		return nil, err
	}
	name, err := models.UniqueEmbedderName(tx, llm.Name, model)
	if err != nil {
		return nil, err
	}
	id := llmID
	e = models.Embedder{Name: name, LLMID: &id, ModelName: model, UserID: userID}
	if err := s.validateEmbedder(tx, &e); err != nil {
		return nil, err
	}
	if err := e.Create(tx); err != nil {
		return nil, err
	}
	s.emitEmbedder(&e, "created", userID)
	return &e, nil
}

// applyHookEmbedEdits re-resolves the embedder of a datasource a plugin hook
// returned modified. Hooks see the legacy embed_* keys; when the plugin
// changed them, the datasource moves to a matching embedder (as a legacy API
// write would). A hook that only changed embedder_id links that embedder.
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
	} else {
		in = EmbedderInput{EmbedderID: embedderIDOrZero(modified.EmbedderID)}
	}
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
