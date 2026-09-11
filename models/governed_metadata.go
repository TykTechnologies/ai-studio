package models

import (
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Governed Metadata (Enterprise)
//
// Governed metadata is admin-defined, validated, typed metadata attached to
// objects that AI Studio publishes to the portal or to edge gateways (LLMs,
// Tools, Datasources, and plugin resource types that opt in). It is distinct
// from the plugin-owned `Metadata JSONMap` on those models, which is a
// free-form, unvalidated bag namespaced per plugin.

// Built-in governed-metadata object types.
const (
	GovernedObjectTypeLLM        = "llm"
	GovernedObjectTypeTool       = "tool"
	GovernedObjectTypeDatasource = "datasource"

	// GovernedObjectTypeAll is the wildcard for MetadataSchema.AppliesTo.
	GovernedObjectTypeAll = "*"

	// GovernedObjectTypePluginResourcePrefix prefixes object types derived from
	// plugin resource types: plugin_resource:<plugin_id>:<slug>.
	GovernedObjectTypePluginResourcePrefix = "plugin_resource:"
)

// Schema enforcement levels.
const (
	MetadataEnforcementAdvisory = "advisory" // validation issues are reported, never block
	MetadataEnforcementEnforce  = "enforce"  // hard errors block admin create/update of the object
)

// Field issue severities.
const (
	MetadataSeverityError   = "error"
	MetadataSeverityWarning = "warning"
)

// Field types supported by governed metadata schemas.
const (
	MetadataFieldTypeString          = "string"
	MetadataFieldTypeText            = "text"
	MetadataFieldTypeNumber          = "number"
	MetadataFieldTypeBoolean         = "boolean"
	MetadataFieldTypeDate            = "date"
	MetadataFieldTypeEmail           = "email"
	MetadataFieldTypeURL             = "url"
	MetadataFieldTypeUser            = "user"
	MetadataFieldTypeVocabulary      = "vocabulary"
	MetadataFieldTypeMultiVocabulary = "multi_vocabulary"
	MetadataFieldTypeStringList      = "string_list"
)

// Validation statuses stored on ObjectMetadata.
const (
	MetadataStatusValid       = "valid"
	MetadataStatusWarnings    = "warnings"
	MetadataStatusInvalid     = "invalid"
	MetadataStatusUnvalidated = "unvalidated"
)

// Sources of schemas, vocabularies and metadata writes.
const (
	MetadataSourceAdmin  = "admin"
	MetadataSourceSystem = "system"
	// Plugin sources are "plugin:<id>"; see MetadataSourcePlugin.
)

// MetadataSourcePlugin builds the source string for a plugin.
func MetadataSourcePlugin(pluginID uint) string {
	return "plugin:" + strconv.FormatUint(uint64(pluginID), 10)
}

// BuiltinObjectID renders a built-in object's numeric ID as a governed-metadata object ID.
func BuiltinObjectID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

// PluginResourceObjectType builds the governed-metadata object type slug for a plugin resource type.
func PluginResourceObjectType(pluginID uint, slug string) string {
	return GovernedObjectTypePluginResourcePrefix + strconv.FormatUint(uint64(pluginID), 10) + ":" + slug
}

// GovernedObjectTypeSelfPrefix lets a plugin refer to its own resource types
// without knowing its numeric plugin ID: "plugin_resource:self:<slug>". It is
// accepted in manifest schemas (applies_to) and in management API calls, and
// rewritten to the concrete object type for the calling plugin.
const GovernedObjectTypeSelfPrefix = GovernedObjectTypePluginResourcePrefix + "self:"

// IsSelfObjectType reports whether objectType uses the "self" plugin placeholder.
func IsSelfObjectType(objectType string) bool {
	return strings.HasPrefix(objectType, GovernedObjectTypeSelfPrefix)
}

// ResolveSelfObjectType rewrites "plugin_resource:self:<slug>" to the object type
// of pluginID's resource type. Any other value is returned unchanged.
func ResolveSelfObjectType(objectType string, pluginID uint) string {
	if !IsSelfObjectType(objectType) {
		return objectType
	}
	return PluginResourceObjectType(pluginID, strings.TrimPrefix(objectType, GovernedObjectTypeSelfPrefix))
}

// MetadataFieldDef is one field in a MetadataSchema.
type MetadataFieldDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`

	Required bool   `json:"required"`
	Severity string `json:"severity,omitempty"` // error (default) | warning
	// RequiredOnPublish makes the field mandatory only when the object goes
	// live (activate / enable): a submitter can save a draft without it, but
	// the publish is refused until it is filled. Independent of Required.
	RequiredOnPublish bool `json:"required_on_publish,omitempty"`

	VocabularySlug string   `json:"vocabulary_slug,omitempty"`
	Pattern        string   `json:"pattern,omitempty"`
	Min            *float64 `json:"min,omitempty"`
	Max            *float64 `json:"max,omitempty"`
	MaxLength      int      `json:"max_length,omitempty"`

	WarnIfPast     bool `json:"warn_if_past,omitempty"` // date fields: warn when the value is in the past
	PortalVisible  bool `json:"portal_visible"`
	GatewayVisible bool `json:"gateway_visible"`
	Order          int  `json:"order"`
}

// MetadataSchema is an admin- or plugin-defined set of governed metadata fields
// that applies to one or more object types.
type MetadataSchema struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"size:255"`
	Slug        string `json:"slug" gorm:"size:100;uniqueIndex:idx_metadata_schema_slug"`
	Description string `json:"description"`

	AppliesTo []string           `json:"applies_to" gorm:"serializer:json"`
	Fields    []MetadataFieldDef `json:"fields" gorm:"serializer:json"`

	Enforcement string `json:"enforcement" gorm:"size:20;default:'advisory'"`
	Active      bool   `json:"active"`
	Source      string `json:"source" gorm:"size:50;default:'admin'"`
	Order       int    `json:"order" gorm:"default:0"`
	Version     int    `json:"version" gorm:"default:1"`
}

type MetadataSchemas []MetadataSchema

func (MetadataSchema) TableName() string { return "metadata_schemas" }

// AppliesToType reports whether the schema applies to the given object type.
func (s *MetadataSchema) AppliesToType(objectType string) bool {
	for _, t := range s.AppliesTo {
		if t == GovernedObjectTypeAll || t == objectType {
			return true
		}
	}
	return false
}

// IsPluginSourced reports whether the schema was contributed by a plugin manifest.
func (s *MetadataSchema) IsPluginSourced() bool {
	return len(s.Source) > len("plugin:") && s.Source[:len("plugin:")] == "plugin:"
}

func (s *MetadataSchema) Create(db *gorm.DB) error { return db.Create(s).Error }
func (s *MetadataSchema) Get(db *gorm.DB, id uint) error {
	return db.First(s, id).Error
}
func (s *MetadataSchema) GetBySlug(db *gorm.DB, slug string) error {
	return db.Where("slug = ?", slug).First(s).Error
}
func (s *MetadataSchema) Update(db *gorm.DB) error { return db.Save(s).Error }
func (s *MetadataSchema) Delete(db *gorm.DB) error { return db.Delete(s).Error }

// GetAll returns all schemas ordered for deterministic resolution.
func (ss *MetadataSchemas) GetAll(db *gorm.DB, activeOnly bool) error {
	q := db.Order("\"order\" ASC, id ASC")
	if activeOnly {
		q = q.Where("active = ?", true)
	}
	return q.Find(ss).Error
}

// VocabularyTerm is one allowed value in a MetadataVocabulary.
type VocabularyTerm struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
}

// MetadataVocabulary is a shared controlled vocabulary referenced by
// vocabulary / multi_vocabulary fields.
type MetadataVocabulary struct {
	gorm.Model
	ID          uint             `json:"id" gorm:"primaryKey"`
	Name        string           `json:"name" gorm:"size:255"`
	Slug        string           `json:"slug" gorm:"size:100;uniqueIndex:idx_metadata_vocab_slug"`
	Description string           `json:"description"`
	Terms       []VocabularyTerm `json:"terms" gorm:"serializer:json"`
	Source      string           `json:"source" gorm:"size:50;default:'admin'"`
}

type MetadataVocabularies []MetadataVocabulary

func (MetadataVocabulary) TableName() string { return "metadata_vocabularies" }

// TermLabel returns the label for a value (or the value itself when unknown).
func (v *MetadataVocabulary) TermLabel(value string) string {
	for _, t := range v.Terms {
		if t.Value == value {
			if t.Label != "" {
				return t.Label
			}
			return t.Value
		}
	}
	return value
}

// HasTerm reports whether the vocabulary contains the value; the second result
// is whether that term is deprecated.
func (v *MetadataVocabulary) HasTerm(value string) (bool, bool) {
	for _, t := range v.Terms {
		if t.Value == value {
			return true, t.Deprecated
		}
	}
	return false, false
}

func (v *MetadataVocabulary) Create(db *gorm.DB) error { return db.Create(v).Error }
func (v *MetadataVocabulary) Get(db *gorm.DB, id uint) error {
	return db.First(v, id).Error
}
func (v *MetadataVocabulary) GetBySlug(db *gorm.DB, slug string) error {
	return db.Where("slug = ?", slug).First(v).Error
}
func (v *MetadataVocabulary) Update(db *gorm.DB) error { return db.Save(v).Error }
func (v *MetadataVocabulary) Delete(db *gorm.DB) error { return db.Delete(v).Error }

func (vs *MetadataVocabularies) GetAll(db *gorm.DB) error {
	return db.Order("name ASC").Find(vs).Error
}

// ObjectMetadata holds the governed metadata values for one object.
type ObjectMetadata struct {
	gorm.Model
	ID         uint   `json:"id" gorm:"primaryKey"`
	ObjectType string `json:"object_type" gorm:"size:200;index:idx_objmeta_type;uniqueIndex:idx_objmeta_type_id"`
	ObjectID   string `json:"object_id" gorm:"size:200;uniqueIndex:idx_objmeta_type_id"`

	Values JSONMap `json:"values" gorm:"type:json"`

	ValidationStatus string     `json:"validation_status" gorm:"size:20;default:'unvalidated'"`
	ValidationResult JSONMap    `json:"validation_result" gorm:"type:json"`
	LastValidatedAt  *time.Time `json:"last_validated_at"`

	UpdatedByUserID uint   `json:"updated_by_user_id"`
	UpdatedBySource string `json:"updated_by_source" gorm:"size:50"`
}

func (ObjectMetadata) TableName() string { return "object_metadata" }

func (o *ObjectMetadata) GetByObject(db *gorm.DB, objectType, objectID string) error {
	return db.Where("object_type = ? AND object_id = ?", objectType, objectID).First(o).Error
}

// ObjectMetadataAudit records every change to an object's governed metadata.
type ObjectMetadataAudit struct {
	gorm.Model
	ID           uint     `json:"id" gorm:"primaryKey"`
	ObjectType   string   `json:"object_type" gorm:"size:200;index:idx_objmeta_audit_obj"`
	ObjectID     string   `json:"object_id" gorm:"size:200;index:idx_objmeta_audit_obj"`
	Action       string   `json:"action" gorm:"size:20"` // set | merge | delete
	UserID       uint     `json:"user_id"`
	Source       string   `json:"source" gorm:"size:50"`
	Before       JSONMap  `json:"before" gorm:"type:json"`
	After        JSONMap  `json:"after" gorm:"type:json"`
	HookExecuted []string `json:"hooks_executed" gorm:"serializer:json"`
}

func (ObjectMetadataAudit) TableName() string { return "object_metadata_audits" }
