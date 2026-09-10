// Package governed_metadata defines the Governed Metadata service used to attach
// admin-defined, validated metadata (owners, lifecycle state, risk tier, data
// classification, ...) to objects that AI Studio publishes to the portal or to
// edge gateways (LLMs, Tools, Datasources, and opted-in plugin resource types).
//
// Community Edition ships a stub that reports no schemas and rejects writes;
// Enterprise Edition registers the full implementation via RegisterEnterpriseFactory.
package governed_metadata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Visibility selects which fields VisibleValues returns.
type Visibility int

const (
	VisibilityAdmin   Visibility = iota // all fields
	VisibilityPortal                    // fields flagged portal_visible
	VisibilityGateway                   // fields flagged gateway_visible
)

// Hook types the service runs on metadata records (object type "governed_metadata").
const (
	HookBeforeUpdate = "before_update"
	HookAfterUpdate  = "after_update"
	HookBeforeDelete = "before_delete"
	HookAfterDelete  = "after_delete"
)

// Event topics emitted after metadata changes.
const (
	TopicUpdated = "system.governed_metadata.updated"
	TopicDeleted = "system.governed_metadata.deleted"
)

var (
	// ErrEnterpriseFeature is returned by write and schema operations in Community Edition.
	ErrEnterpriseFeature = errors.New("governed metadata is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")
	// ErrNotFound is returned when a schema, vocabulary or metadata record does not exist.
	ErrNotFound = errors.New("not found")
	// ErrVocabularyInUse is returned when deleting a vocabulary referenced by an active schema field.
	ErrVocabularyInUse = errors.New("vocabulary is referenced by one or more schema fields")
	// ErrReadOnlySchema is returned when editing structural fields of a plugin-sourced schema.
	ErrReadOnlySchema = errors.New("plugin-sourced schemas can only have active and enforcement changed")
	// ErrInvalidObjectType is returned when an object type is unknown.
	ErrInvalidObjectType = errors.New("unknown object type")
)

// SchemaKeyCollisionError is returned when a field key is already defined by another
// active schema that overlaps in applies_to.
type SchemaKeyCollisionError struct {
	Key        string
	OtherSlug  string
	ObjectType string
}

func (e *SchemaKeyCollisionError) Error() string {
	return fmt.Sprintf("field key %q collides with schema %q for object type %q", e.Key, e.OtherSlug, e.ObjectType)
}

// SchemaDefinitionError is returned when a schema or vocabulary definition is malformed.
type SchemaDefinitionError struct {
	Field   string
	Message string
}

func (e *SchemaDefinitionError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationError is returned by SetObjectMetadata when enforcement is on and
// the values have hard errors.
type ValidationError struct {
	Result *ValidationResult
}

func (e *ValidationError) Error() string {
	if e.Result == nil || len(e.Result.Errors) == 0 {
		return "governed metadata validation failed"
	}
	return fmt.Sprintf("governed metadata validation failed: %s", e.Result.Errors[0].Message)
}

// HookRejectedError is returned when a plugin object hook rejects a metadata change.
type HookRejectedError struct {
	Reason string
}

func (e *HookRejectedError) Error() string {
	return fmt.Sprintf("metadata change rejected by plugin: %s", e.Reason)
}

// FieldIssue is one validation error or warning.
type FieldIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationResult is the standard validation envelope.
type ValidationResult struct {
	Valid    bool         `json:"valid"`    // no hard errors
	Enforced bool         `json:"enforced"` // resolved schema enforcement == enforce
	Errors   []FieldIssue `json:"errors"`
	Warnings []FieldIssue `json:"warnings"`
}

// Status derives the stored validation status from the result.
func (r *ValidationResult) Status() string {
	switch {
	case r == nil:
		return models.MetadataStatusUnvalidated
	case len(r.Errors) > 0:
		return models.MetadataStatusInvalid
	case len(r.Warnings) > 0:
		return models.MetadataStatusWarnings
	default:
		return models.MetadataStatusValid
	}
}

// ResolvedSchema is the merged view of every active schema that applies to an object type.
type ResolvedSchema struct {
	ObjectType  string                    `json:"object_type"`
	Fields      []models.MetadataFieldDef `json:"fields"`
	JSONSchema  map[string]interface{}    `json:"json_schema"`
	Enforcement string                    `json:"enforcement"`
	SchemaSlugs []string                  `json:"schema_slugs"`
	// Vocabularies holds the terms of every vocabulary referenced by Fields,
	// keyed by slug, so a consumer can render selects with labels without a
	// second round-trip (plugins only see this via the management API).
	Vocabularies map[string][]models.VocabularyTerm `json:"vocabularies"`
}

// HasFields reports whether any field applies.
func (r *ResolvedSchema) HasFields() bool { return r != nil && len(r.Fields) > 0 }

// IsEnforced reports whether hard errors block writes.
func (r *ResolvedSchema) IsEnforced() bool {
	return r != nil && r.Enforcement == models.MetadataEnforcementEnforce
}

// ObjectTypeInfo describes an object type that can carry governed metadata.
type ObjectTypeInfo struct {
	Slug   string `json:"slug"`
	Label  string `json:"label"`
	Source string `json:"source"` // builtin | plugin:<id>
}

// DisplayField is the portal-facing, display-ready form of one value.
type DisplayField struct {
	Key   string      `json:"key"`
	Label string      `json:"label"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// SetOptions controls SetObjectMetadata.
type SetOptions struct {
	Merge           bool   // merge into existing values instead of replacing
	UserID          uint   // acting user (0 = system/plugin)
	Source          string // admin | system | plugin:<id>
	SkipEnforcement bool   // values were already validated by the caller; still records status
}

// ComplianceFilter narrows ComplianceReport.
type ComplianceFilter struct {
	ObjectType string // empty = all
	Status     string // empty = all; missing | invalid | warnings | expired | valid
}

// ComplianceEntry is one row in the compliance report.
type ComplianceEntry struct {
	ObjectType      string       `json:"object_type"`
	ObjectID        string       `json:"object_id"`
	ObjectName      string       `json:"object_name"`
	Status          string       `json:"status"` // missing | invalid | warnings | expired | valid
	Issues          []FieldIssue `json:"issues"`
	LastValidatedAt *time.Time   `json:"last_validated_at,omitempty"`
}

// ComplianceReport aggregates compliance entries.
type ComplianceReport struct {
	Entries []ComplianceEntry `json:"entries"`
	Counts  map[string]int    `json:"counts"` // status -> count
}

// HookOutcome is the result of running object hooks on a metadata record.
type HookOutcome struct {
	Allowed         bool
	RejectionReason string
	Modified        *models.ObjectMetadata // non-nil when a plugin modified the record
	Executed        []string
}

// HookRunner runs plugin object hooks for governed metadata records.
// Implemented in the services package; nil when no plugin manager is available.
type HookRunner interface {
	RunMetadataHook(ctx context.Context, hookType string, rec *models.ObjectMetadata, userID uint) (*HookOutcome, error)
}

// EventEmitter publishes system events. Satisfied by *services.SystemEventEmitter.
type EventEmitter interface {
	EmitObjectEvent(topic, objectType, action string, objectID uint, userID uint, object interface{})
}

// NamedInstance is a plugin resource instance as seen by the compliance report.
type NamedInstance struct {
	ID   string
	Name string
}

// ResourceInstanceLister enumerates the live instances of a plugin resource type
// (they are owned by the plugin, not stored in Studio). nil = plugin instances
// are left out of the compliance report.
type ResourceInstanceLister func(pluginID uint, slug string) ([]NamedInstance, error)

// Deps are the collaborators the service needs. Events is resolved lazily
// because the system event emitter is attached after service construction.
type Deps struct {
	Hooks             HookRunner
	Events            func() EventEmitter
	ResourceInstances ResourceInstanceLister
}

// SnapshotReader is the subset used when building edge configuration snapshots.
type SnapshotReader interface {
	ListObjectMetadata(objectType string, objectIDs []string) (map[string]*models.ObjectMetadata, error)
	VisibleValues(objectType string, rec *models.ObjectMetadata, vis Visibility) map[string]interface{}
}

// Service is the Governed Metadata service.
type Service interface {
	SnapshotReader

	// --- Schemas ---

	// ListSchemas returns all schemas (active and inactive).
	// CE: Returns an empty list.
	// ENT: Returns schemas ordered by order, id.
	ListSchemas() ([]models.MetadataSchema, error)

	// GetSchema returns one schema.
	// CE: Returns ErrEnterpriseFeature
	GetSchema(id uint) (*models.MetadataSchema, error)

	// CreateSchema validates and stores a schema.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Validates field definitions, vocabulary references and key collisions.
	CreateSchema(schema *models.MetadataSchema) error

	// UpdateSchema validates and stores changes to a schema; bumps Version.
	// CE: Returns ErrEnterpriseFeature
	UpdateSchema(schema *models.MetadataSchema) error

	// DeleteSchema removes a schema. Existing object values are kept.
	// CE: Returns ErrEnterpriseFeature
	DeleteSchema(id uint) error

	// --- Vocabularies ---

	ListVocabularies() ([]models.MetadataVocabulary, error)
	GetVocabulary(id uint) (*models.MetadataVocabulary, error)
	CreateVocabulary(vocab *models.MetadataVocabulary) error
	UpdateVocabulary(vocab *models.MetadataVocabulary) error
	// DeleteVocabulary refuses with ErrVocabularyInUse when referenced by a schema field.
	DeleteVocabulary(id uint) error

	// --- Object types and resolution ---

	// ListObjectTypes returns built-in types plus plugin resource types that opted in.
	// CE: Returns the built-in types only.
	ListObjectTypes() ([]ObjectTypeInfo, error)

	// ResolveSchema merges every active schema that applies to the object type.
	// CE: Returns an empty resolved schema (no fields, advisory).
	ResolveSchema(objectType string) (*ResolvedSchema, error)

	// Validate checks values against the resolved schema.
	// CE: Returns {Valid: true}.
	Validate(objectType string, values map[string]interface{}) (*ValidationResult, error)

	// --- Values ---

	// GetObjectMetadata returns the stored record, or ErrNotFound.
	// CE: Returns ErrNotFound.
	GetObjectMetadata(objectType, objectID string) (*models.ObjectMetadata, error)

	// SetObjectMetadata validates, runs hooks, stores values and audit, and emits an event.
	// Returns *ValidationError when enforcement is on and values have hard errors,
	// or *HookRejectedError when a plugin rejects the change.
	// CE: Returns ErrEnterpriseFeature
	SetObjectMetadata(ctx context.Context, objectType, objectID string, values map[string]interface{}, opts SetOptions) (*models.ObjectMetadata, *ValidationResult, error)

	// DeleteObjectMetadata removes the record for an object (no error when absent).
	// CE: Returns nil.
	DeleteObjectMetadata(ctx context.Context, objectType, objectID string, opts SetOptions) error

	// ListAudit returns the audit trail for an object, newest first.
	// CE: Returns an empty list.
	ListAudit(objectType, objectID string, limit int) ([]models.ObjectMetadataAudit, error)

	// DisplayValues renders portal-visible values with labels resolved.
	// CE: Returns nil.
	DisplayValues(objectType string, rec *models.ObjectMetadata) []DisplayField

	// --- Reporting, plugins, seeding ---

	// ComplianceReport lists objects with missing, invalid, warning or expired metadata.
	// CE: Returns ErrEnterpriseFeature
	ComplianceReport(filter ComplianceFilter) (*ComplianceReport, error)

	// UpsertPluginSchemas registers vocabularies and schemas declared in a plugin manifest.
	// CE: Returns ErrEnterpriseFeature
	UpsertPluginSchemas(pluginID uint, manifest *models.ManifestMetadata) error

	// EnsureDefaults seeds the default vocabularies and the "Governance Core" schema
	// when no schemas exist yet.
	// CE: Returns nil.
	EnsureDefaults() error
}
