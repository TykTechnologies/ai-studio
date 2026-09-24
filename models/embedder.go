package models

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// Embedder is a reusable embedding configuration: the client (API
// compatibility), endpoint, credentials and model used to turn text into
// vectors. Datasources and Semantic Routers reference one by id.
//
// An Embedder is either linked to an LLM, in which case the vendor, endpoint,
// key and privacy score come from that LLM live, or standalone, in which case
// it carries its own. Runtime code never reads these fields directly: it asks
// for a resolved EmbedderSpec (Spec, GetEmbedderResolved).
type Embedder struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"uniqueIndex:idx_embedder_name;not null"`
	Description string `json:"description"`

	// LLMID links the embedder to an LLM. When set, Vendor, Endpoint, APIKey
	// and PrivacyScore are ignored in favour of the LLM's.
	LLMID *uint `json:"llm_id" gorm:"index"`
	LLM   *LLM  `json:"-" gorm:"foreignKey:LLMID"`

	// Vendor is the client used to call the endpoint (its API compatibility),
	// not necessarily who serves the model.
	Vendor Vendor `json:"vendor"`
	// Endpoint is the base URL. For Vertex it is "project:location".
	Endpoint string `json:"endpoint"`
	// APIKey is a plain value or a $SECRET/ or $ENV/ reference, as on LLMs.
	APIKey string `json:"api_key"`
	// ModelName is the embedding model (column "model"; gorm.Model owns the
	// Go name).
	ModelName    string `json:"model" gorm:"column:model;not null"`
	PrivacyScore int    `json:"privacy_score"`

	UserID uint `json:"user_id"`
}

// Embedders is a list of embedders.
type Embedders []Embedder

// EmbedderSpec is an embedder resolved to what a client needs: a linked
// embedder's LLM fields are filled in, and secret references resolved when
// asked. It is the only shape runtime embedding code consumes.
type EmbedderSpec struct {
	EmbedderID   uint
	Vendor       Vendor
	Endpoint     string
	APIKey       string
	Model        string
	PrivacyScore int
}

var (
	// ErrEmbedderLLMMissing is returned when a linked embedder's LLM is gone.
	ErrEmbedderLLMMissing = errors.New("embedder's linked LLM no longer exists")
	// ErrEmbedderInvalid wraps structural validation failures.
	ErrEmbedderInvalid = errors.New("invalid embedder")
)

// IsLinked reports whether the embedder takes its connection from an LLM.
func (e *Embedder) IsLinked() bool { return e.LLMID != nil && *e.LLMID != 0 }

// Validate checks the embedder's shape. Whether the vendor actually serves
// embeddings is checked by the service layer (it needs the vendor drivers).
func (e *Embedder) Validate() error {
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrEmbedderInvalid)
	}
	if strings.TrimSpace(e.ModelName) == "" {
		return fmt.Errorf("%w: model is required", ErrEmbedderInvalid)
	}
	if e.IsLinked() {
		if e.Vendor != "" || e.Endpoint != "" || e.APIKey != "" {
			return fmt.Errorf("%w: a linked embedder takes its vendor, endpoint and key from the LLM", ErrEmbedderInvalid)
		}
		return nil
	}
	if e.Vendor == "" {
		return fmt.Errorf("%w: vendor (API compatibility) is required", ErrEmbedderInvalid)
	}
	if e.PrivacyScore < 0 || e.PrivacyScore > 100 {
		return fmt.Errorf("%w: privacy score must be between 0 and 100", ErrEmbedderInvalid)
	}
	return nil
}

// Spec resolves the embedder. A linked embedder needs its LLM preloaded.
// With resolveSecrets, $SECRET/ and $ENV/ references in the key and endpoint
// are replaced by their values; without, they are returned as stored.
func (e *Embedder) Spec(resolveSecrets bool) (*EmbedderSpec, error) {
	s := &EmbedderSpec{EmbedderID: e.ID, Model: e.ModelName}
	if e.IsLinked() {
		if e.LLM == nil || e.LLM.ID == 0 || e.LLM.DeletedAt.Valid {
			return nil, ErrEmbedderLLMMissing
		}
		s.Vendor = e.LLM.Vendor
		s.Endpoint = e.LLM.APIEndpoint
		s.APIKey = e.LLM.APIKey
		s.PrivacyScore = e.LLM.PrivacyScore
	} else {
		s.Vendor = e.Vendor
		s.Endpoint = e.Endpoint
		s.APIKey = e.APIKey
		s.PrivacyScore = e.PrivacyScore
	}
	if resolveSecrets {
		s.APIKey = secrets.GetValue(s.APIKey, false)
		s.Endpoint = secrets.GetValue(s.Endpoint, false)
	}
	return s, nil
}

// EffectivePrivacyScore is the score the embedder is trusted with: its own
// for a standalone embedder, the LLM's for a linked one (0 if the LLM is not
// loaded or gone).
func (e *Embedder) EffectivePrivacyScore() int {
	if e.IsLinked() {
		if e.LLM == nil {
			return 0
		}
		return e.LLM.PrivacyScore
	}
	return e.PrivacyScore
}

// Get loads an embedder by id with its LLM.
func (e *Embedder) Get(db *gorm.DB, id uint) error {
	return db.Preload("LLM").First(e, id).Error
}

// Create inserts the embedder.
func (e *Embedder) Create(db *gorm.DB) error {
	return db.Omit("LLM").Create(e).Error
}

// Update saves the embedder's own fields.
func (e *Embedder) Update(db *gorm.DB) error {
	return db.Omit("LLM", "CreatedAt").Save(e).Error
}

// Delete removes the embedder for good. It is a hard delete: a soft-deleted
// row would keep its name in the unique index. Callers check references first.
func (e *Embedder) Delete(db *gorm.DB) error {
	return db.Unscoped().Delete(&Embedder{}, e.ID).Error
}

// GetAll lists embedders with their LLMs, paged unless all is set.
func (es *Embedders) GetAll(db *gorm.DB, pageSize, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) (int64, int, error) {
	var total int64
	q := db.Model(&Embedder{})
	for _, s := range scopes {
		q = s(q)
	}
	if err := q.Count(&total).Error; err != nil {
		return 0, 0, err
	}
	totalPages := 1
	q = q.Preload("LLM")
	if !all && pageSize > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
		if pageNumber < 1 {
			pageNumber = 1
		}
		q = q.Offset((pageNumber - 1) * pageSize).Limit(pageSize)
	}
	if err := q.Find(es).Error; err != nil {
		return 0, 0, err
	}
	return total, totalPages, nil
}

// GetEmbedderResolved loads an embedder and resolves it, secrets included.
// This is how runtime code obtains an embedding configuration by id.
func GetEmbedderResolved(db *gorm.DB, id uint) (*EmbedderSpec, error) {
	var e Embedder
	if err := e.Get(db, id); err != nil {
		return nil, err
	}
	return e.Spec(true)
}

// FindOrCreateLinkedEmbedder returns the embedder linked to the LLM with this
// model, creating one (named "<LLM name> · <model>") when there is none.
// created reports whether it was made now.
//
// Concurrent calls for the same LLM and model create one embedder (see
// LockEmbedderConfig).
func FindOrCreateLinkedEmbedder(db *gorm.DB, llmID uint, model string, userID uint) (e *Embedder, created bool, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := LockEmbedderConfig(tx, "linked", fmt.Sprint(llmID), model); err != nil {
			return err
		}
		var found Embedder
		ferr := tx.Preload("LLM").Where("llm_id = ? AND model = ?", llmID, model).Order("id").First(&found).Error
		if ferr == nil {
			e = &found
			return nil
		}
		if !errors.Is(ferr, gorm.ErrRecordNotFound) {
			return ferr
		}
		var llm LLM
		if err := tx.First(&llm, llmID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: LLM %d does not exist", ErrEmbedderInvalid, llmID)
			}
			return err
		}
		id := llmID
		found = Embedder{LLMID: &id, ModelName: model, UserID: userID}
		if err := CreateWithDefaultName(tx, &found, llm.Name, model); err != nil {
			return err
		}
		found.LLM = &llm
		e, created = &found, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return e, created, nil
}

// ErrEmbedderNamespace is returned when a linked embedder's LLM is scoped to
// a namespace other than its consumer's.
var ErrEmbedderNamespace = errors.New("embedder is not available in this namespace")

// UsableInNamespace reports whether the embedder may serve a datasource or
// router in namespace ns. A linked embedder carries its LLM's credentials,
// and edges only receive an LLM in its own namespace (or everywhere when it
// is global): so an LLM scoped to a namespace may only embed for objects in
// that namespace. A standalone embedder has no namespace. The LLM must be
// loaded for a linked embedder.
func (e *Embedder) UsableInNamespace(ns string) error {
	if !e.IsLinked() || e.LLM == nil {
		return nil
	}
	llmNS := CanonicalNamespace(e.LLM.Namespace)
	if llmNS == DefaultNamespace || llmNS == CanonicalNamespace(ns) {
		return nil
	}
	return fmt.Errorf("%w: it uses LLM %q, which is scoped to namespace %q", ErrEmbedderNamespace, e.LLM.Name, e.LLM.Namespace)
}
