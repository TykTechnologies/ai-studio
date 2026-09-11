package governed_metadata

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// communityService is the Community Edition stub. Reads report "nothing
// configured" so serializers and snapshots are no-ops; writes and schema
// management return ErrEnterpriseFeature.
type communityService struct{}

func newCommunityService() Service { return &communityService{} }

func (s *communityService) ListSchemas() ([]models.MetadataSchema, error) {
	return []models.MetadataSchema{}, nil
}
func (s *communityService) GetSchema(id uint) (*models.MetadataSchema, error) {
	return nil, ErrEnterpriseFeature
}
func (s *communityService) CreateSchema(schema *models.MetadataSchema) error {
	return ErrEnterpriseFeature
}
func (s *communityService) UpdateSchema(schema *models.MetadataSchema) error {
	return ErrEnterpriseFeature
}
func (s *communityService) DeleteSchema(id uint) error { return ErrEnterpriseFeature }

func (s *communityService) ListVocabularies() ([]models.MetadataVocabulary, error) {
	return []models.MetadataVocabulary{}, nil
}
func (s *communityService) GetVocabulary(id uint) (*models.MetadataVocabulary, error) {
	return nil, ErrEnterpriseFeature
}
func (s *communityService) CreateVocabulary(v *models.MetadataVocabulary) error {
	return ErrEnterpriseFeature
}
func (s *communityService) UpdateVocabulary(v *models.MetadataVocabulary) error {
	return ErrEnterpriseFeature
}
func (s *communityService) DeleteVocabulary(id uint) error { return ErrEnterpriseFeature }

func (s *communityService) ListObjectTypes() ([]ObjectTypeInfo, error) {
	return BuiltinObjectTypes(), nil
}

func (s *communityService) ResolveSchema(objectType string) (*ResolvedSchema, error) {
	return &ResolvedSchema{
		ObjectType:  objectType,
		Fields:      []models.MetadataFieldDef{},
		JSONSchema:  map[string]interface{}{"type": "object"},
		Enforcement: models.MetadataEnforcementAdvisory,
		SchemaSlugs: []string{},
	}, nil
}

func (s *communityService) ValidateForPublish(context.Context, string, string, map[string]interface{}) (*ValidationResult, error) {
	return &ValidationResult{Valid: true, Errors: []FieldIssue{}, Warnings: []FieldIssue{}}, nil
}

func (s *communityService) Validate(objectType string, values map[string]interface{}) (*ValidationResult, error) {
	return &ValidationResult{Valid: true, Errors: []FieldIssue{}, Warnings: []FieldIssue{}}, nil
}

func (s *communityService) GetObjectMetadata(objectType, objectID string) (*models.ObjectMetadata, error) {
	return nil, ErrNotFound
}

func (s *communityService) SetObjectMetadata(ctx context.Context, objectType, objectID string, values map[string]interface{}, opts SetOptions) (*models.ObjectMetadata, *ValidationResult, error) {
	return nil, nil, ErrEnterpriseFeature
}

func (s *communityService) DeleteObjectMetadata(ctx context.Context, objectType, objectID string, opts SetOptions) error {
	return nil
}

func (s *communityService) ListObjectMetadata(objectType string, objectIDs []string) (map[string]*models.ObjectMetadata, error) {
	return map[string]*models.ObjectMetadata{}, nil
}

func (s *communityService) ListAudit(objectType, objectID string, limit int) ([]models.ObjectMetadataAudit, error) {
	return []models.ObjectMetadataAudit{}, nil
}

func (s *communityService) VisibleValues(objectType string, rec *models.ObjectMetadata, vis Visibility) map[string]interface{} {
	return nil
}

func (s *communityService) DisplayValues(objectType string, rec *models.ObjectMetadata) []DisplayField {
	return nil
}

func (s *communityService) ComplianceReport(filter ComplianceFilter) (*ComplianceReport, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) UpsertPluginSchemas(pluginID uint, manifest *models.ManifestMetadata) error {
	return ErrEnterpriseFeature
}

func (s *communityService) EnsureDefaults() error { return nil }
