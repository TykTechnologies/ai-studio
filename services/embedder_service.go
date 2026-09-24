package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"gorm.io/gorm"
)

// RedactedEmbedderKey is what a client sends back for an embedder key it was
// shown redacted; it keeps the stored key.
const RedactedEmbedderKey = "[redacted]"

var (
	// ErrEmbedderNotFound is returned for an unknown embedder id.
	ErrEmbedderNotFound = errors.New("embedder not found")
	// ErrEmbedderInvalid is returned for an embedder that fails validation.
	ErrEmbedderInvalid = models.ErrEmbedderInvalid
)

// EmbedderInUseError refuses a delete while other objects embed with the
// embedder.
type EmbedderInUseError struct {
	Dependents *Dependents
}

func (e *EmbedderInUseError) Error() string {
	return fmt.Sprintf("embedder is used by %s; point them at another embedder first", describeDependents(e.Dependents))
}

// EmbedderLockedError refuses a change to the model or API compatibility of
// an embedder that datasources use: their stored vectors came from the
// current model, so new queries embedded another way would not match them.
type EmbedderLockedError struct {
	Datasources []DependentRef
}

func (e *EmbedderLockedError) Error() string {
	return fmt.Sprintf("the model and API compatibility of this embedder cannot change while datasources use it (%s): "+
		"their vectors were made with the current model. Create a new embedder, point the datasources at it and re-process their embeddings",
		refNames(e.Datasources))
}

// EmbedderPrivacyError refuses a pairing where the embedder is trusted with
// less than the data it would see.
type EmbedderPrivacyError struct {
	EmbedderScore int
	Required      int
	Datasource    string
}

func (e *EmbedderPrivacyError) Error() string {
	if e.Datasource != "" {
		return fmt.Sprintf("embedder privacy score %d is below datasource %q's privacy score %d", e.EmbedderScore, e.Datasource, e.Required)
	}
	return fmt.Sprintf("embedder privacy score %d is below the datasource privacy score %d", e.EmbedderScore, e.Required)
}

// LLMEmbedderConflictError refuses an LLM change or delete that would break
// embedders linked to it.
type LLMEmbedderConflictError struct {
	Reason    string
	Embedders []DependentRef
}

func (e *LLMEmbedderConflictError) Error() string {
	return fmt.Sprintf("%s: embedders linked to this LLM (%s) depend on it", e.Reason, refNames(e.Embedders))
}

func refNames(refs []DependentRef) string {
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.Name)
	}
	return strings.Join(names, ", ")
}

func describeDependents(d *Dependents) string {
	if d == nil {
		return "other objects"
	}
	var parts []string
	if n := len(d.Datasources); n > 0 {
		parts = append(parts, plural(n, "datasource")+" ("+refNames(d.Datasources)+")")
	}
	if n := len(d.SemanticRouters); n > 0 {
		parts = append(parts, plural(n, "semantic router")+" ("+refNames(d.SemanticRouters)+")")
	}
	if len(parts) == 0 {
		return "other objects"
	}
	return strings.Join(parts, " and ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// validateEmbedder checks the embedder's shape, that its name is free and
// that its vendor (its own, or its linked LLM's) can embed. A linked
// embedder's LLM is loaded onto it.
func (s *Service) validateEmbedder(db *gorm.DB, e *models.Embedder) error {
	if err := e.Validate(); err != nil {
		return err
	}
	var taken int64
	if err := db.Model(&models.Embedder{}).Where("name = ? AND id <> ?", e.Name, e.ID).Count(&taken).Error; err != nil {
		return err
	}
	if taken > 0 {
		return fmt.Errorf("%w: an embedder named %q already exists", ErrEmbedderInvalid, e.Name)
	}
	vendor := e.Vendor
	if e.IsLinked() {
		var llm models.LLM
		if err := db.First(&llm, *e.LLMID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: linked LLM %d does not exist", ErrEmbedderInvalid, *e.LLMID)
			}
			return err
		}
		e.LLM = &llm
		vendor = llm.Vendor
	}
	if !switches.SupportsEmbeddings(vendor) {
		return fmt.Errorf("%w: vendor %q does not provide embeddings", ErrEmbedderInvalid, vendor)
	}
	return nil
}

// GetEmbedder returns an embedder with its LLM loaded.
func (s *Service) GetEmbedder(id uint) (*models.Embedder, error) {
	var e models.Embedder
	if err := e.Get(s.DB, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEmbedderNotFound
		}
		return nil, err
	}
	return &e, nil
}

// GetEmbedderResolved returns an embedder's resolved spec (secrets included)
// for runtime use.
func (s *Service) GetEmbedderResolved(id uint) (*models.EmbedderSpec, error) {
	e, err := s.GetEmbedder(id)
	if err != nil {
		return nil, err
	}
	return e.Spec(true)
}

// ListEmbedders lists embedders with their LLMs.
func (s *Service) ListEmbedders(pageSize, pageNumber int, all bool, opts ...ListOptions) (models.Embedders, int64, int, error) {
	var es models.Embedders
	total, pages, err := es.GetAll(s.DB, pageSize, pageNumber, all,
		firstListOptions(opts).Scopes("name", "description", "model")...)
	if err != nil {
		return nil, 0, 0, err
	}
	return es, total, pages, nil
}

// CreateEmbedder validates and stores a new embedder.
func (s *Service) CreateEmbedder(e *models.Embedder, userID uint) (*models.Embedder, error) {
	e.ID = 0
	e.UserID = userID
	if err := s.validateEmbedder(s.DB, e); err != nil {
		return nil, err
	}
	if err := e.Create(s.DB); err != nil {
		return nil, err
	}
	s.emitEmbedder(e, "created", userID)
	return e, nil
}

// UpdateEmbedder applies changes to an embedder. The key is kept when the
// caller sends it back redacted. The model and API compatibility (vendor, or
// the linked LLM) cannot change while datasources use the embedder, and the
// privacy score cannot drop below theirs.
func (s *Service) UpdateEmbedder(id uint, changes *models.Embedder, userID uint) (*models.Embedder, error) {
	current, err := s.GetEmbedder(id)
	if err != nil {
		return nil, err
	}
	next := *current
	next.Name = changes.Name
	next.Description = changes.Description
	next.LLMID = changes.LLMID
	next.LLM = nil
	next.Vendor = changes.Vendor
	next.Endpoint = changes.Endpoint
	if changes.APIKey != RedactedEmbedderKey {
		next.APIKey = changes.APIKey
	}
	next.ModelName = changes.ModelName
	next.PrivacyScore = changes.PrivacyScore
	if next.IsLinked() {
		// A linked embedder carries no connection fields of its own.
		next.Vendor, next.Endpoint, next.APIKey = "", "", ""
	}

	if err := s.validateEmbedder(s.DB, &next); err != nil {
		return nil, err
	}

	datasources, err := dependentRefs(s.DB, &models.Datasource{}, "datasources", "",
		"datasources.embedder_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(datasources) > 0 {
		if embeddingIdentityChanged(current, &next) {
			return nil, &EmbedderLockedError{Datasources: datasources}
		}
		if err := s.checkEmbedderPrivacyForDatasources(id, next.EffectivePrivacyScore()); err != nil {
			return nil, err
		}
	}

	if err := next.Update(s.DB); err != nil {
		return nil, err
	}
	s.emitEmbedder(&next, "updated", userID)
	return &next, nil
}

// embeddingIdentityChanged reports whether the change would make the
// embedder produce vectors in a different space: another model, another
// client, or another LLM (which may be another vendor).
func embeddingIdentityChanged(a, b *models.Embedder) bool {
	if a.ModelName != b.ModelName || a.IsLinked() != b.IsLinked() {
		return true
	}
	if a.IsLinked() {
		return *a.LLMID != *b.LLMID
	}
	return a.Vendor != b.Vendor
}

// checkEmbedderPrivacyForDatasources refuses a score below any datasource
// that embeds with the embedder.
func (s *Service) checkEmbedderPrivacyForDatasources(embedderID uint, score int) error {
	var ds models.Datasource
	err := s.DB.Where("embedder_id = ?", embedderID).Order("privacy_score DESC").First(&ds).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if score < ds.PrivacyScore {
		return &EmbedderPrivacyError{EmbedderScore: score, Required: ds.PrivacyScore, Datasource: ds.Name}
	}
	return nil
}

// DeleteEmbedder removes an embedder nothing uses.
func (s *Service) DeleteEmbedder(id uint, userID uint) error {
	e, err := s.GetEmbedder(id)
	if err != nil {
		return err
	}
	deps, err := s.GetEmbedderDependents(id)
	if err != nil {
		return err
	}
	if deps.Total > 0 {
		return &EmbedderInUseError{Dependents: deps}
	}
	if err := e.Delete(s.DB); err != nil {
		return err
	}
	if s.SystemEvents != nil {
		s.SystemEvents.EmitEmbedderDeleted(id, userID)
	}
	return nil
}

// RedactedEmbedder is an embedder as events and API responses carry it.
func RedactedEmbedder(e *models.Embedder) *models.Embedder {
	c := *e
	if c.APIKey != "" && !secrets.IsSecretReference(c.APIKey) {
		c.APIKey = RedactedEmbedderKey
	}
	return &c
}

func (s *Service) emitEmbedder(e *models.Embedder, action string, userID uint) {
	if s.SystemEvents == nil {
		return
	}
	r := RedactedEmbedder(e)
	switch action {
	case "created":
		s.SystemEvents.EmitEmbedderCreated(r, e.ID, userID)
	case "updated":
		s.SystemEvents.EmitEmbedderUpdated(r, e.ID, userID)
	}
}

// linkedEmbedderRefs lists the embedders linked to an LLM.
func (s *Service) linkedEmbedderRefs(llmID uint) ([]DependentRef, error) {
	return dependentRefs(s.DB, &models.Embedder{}, "embedders", "", "embedders.llm_id = ?", llmID)
}

// CheckLLMDeleteForEmbedders refuses deleting an LLM that embedders link to.
func (s *Service) CheckLLMDeleteForEmbedders(llmID uint) error {
	refs, err := s.linkedEmbedderRefs(llmID)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		return &LLMEmbedderConflictError{Reason: "cannot delete this LLM", Embedders: refs}
	}
	return nil
}

// CheckLLMUpdateForEmbedders refuses an LLM change that would break the
// datasources embedding through it: a vendor its drivers cannot embed with
// or a different vendor (another vector space), or a privacy score below
// theirs.
func (s *Service) CheckLLMUpdateForEmbedders(current *models.LLM, newVendor models.Vendor, newPrivacy int) error {
	refs, err := s.linkedEmbedderRefs(current.ID)
	if err != nil || len(refs) == 0 {
		return err
	}
	if newVendor != current.Vendor && !switches.SupportsEmbeddings(newVendor) {
		return &LLMEmbedderConflictError{Reason: fmt.Sprintf("vendor %q does not provide embeddings", newVendor), Embedders: refs}
	}

	type row struct {
		Name         string
		PrivacyScore int
	}
	var ds []row
	if err := s.DB.Table("datasources").
		Select("datasources.name, datasources.privacy_score").
		Joins("JOIN embedders ON embedders.id = datasources.embedder_id").
		Where("embedders.llm_id = ? AND datasources.deleted_at IS NULL", current.ID).
		Order("datasources.privacy_score DESC").
		Scan(&ds).Error; err != nil {
		return err
	}
	if len(ds) == 0 {
		return nil
	}
	if newVendor != current.Vendor {
		return &LLMEmbedderConflictError{Reason: "the vendor cannot change while datasources embed through this LLM", Embedders: refs}
	}
	if newPrivacy < ds[0].PrivacyScore {
		return &EmbedderPrivacyError{EmbedderScore: newPrivacy, Required: ds[0].PrivacyScore, Datasource: ds[0].Name}
	}
	return nil
}

// EmbeddingVendors lists the vendors whose drivers can embed.
func (s *Service) EmbeddingVendors() []models.Vendor {
	return switches.EmbeddingVendors()
}
