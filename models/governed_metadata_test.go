package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGovernedMetadataIdentifiers(t *testing.T) {
	assert.Equal(t, "42", BuiltinObjectID(42))
	assert.Equal(t, "plugin:7", MetadataSourcePlugin(7))
	assert.Equal(t, "plugin_resource:7:widgets", PluginResourceObjectType(7, "widgets"))
}

func TestMetadataSchemaHelpers(t *testing.T) {
	wildcard := MetadataSchema{AppliesTo: []string{"*"}}
	assert.True(t, wildcard.AppliesToType("llm"))
	assert.True(t, wildcard.AppliesToType("plugin_resource:1:x"))

	scoped := MetadataSchema{AppliesTo: []string{"tool", "datasource"}}
	assert.True(t, scoped.AppliesToType("tool"))
	assert.False(t, scoped.AppliesToType("llm"))
	empty := MetadataSchema{}
	assert.False(t, empty.AppliesToType("llm"))

	assert.True(t, (&MetadataSchema{Source: "plugin:3"}).IsPluginSourced())
	assert.False(t, (&MetadataSchema{Source: "plugin:"}).IsPluginSourced(), "a bare prefix is not a plugin id")
	assert.False(t, (&MetadataSchema{Source: "admin"}).IsPluginSourced())
	assert.False(t, (&MetadataSchema{}).IsPluginSourced())
}

func TestMetadataVocabularyHelpers(t *testing.T) {
	v := MetadataVocabulary{Terms: []VocabularyTerm{
		{Value: "high", Label: "High"},
		{Value: "legacy", Deprecated: true},
	}}
	assert.Equal(t, "High", v.TermLabel("high"))
	assert.Equal(t, "legacy", v.TermLabel("legacy"), "falls back to the value when no label is set")
	assert.Equal(t, "unknown", v.TermLabel("unknown"))

	found, deprecated := v.HasTerm("legacy")
	assert.True(t, found)
	assert.True(t, deprecated)
	found, deprecated = v.HasTerm("high")
	assert.True(t, found)
	assert.False(t, deprecated)
	found, _ = v.HasTerm("nope")
	assert.False(t, found)
}

func TestGovernedMetadataPersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&MetadataSchema{}, &MetadataVocabulary{}, &ObjectMetadata{}, &ObjectMetadataAudit{}))

	// Active defaults to false and round-trips explicitly (no gorm default tag).
	inactive := &MetadataSchema{Name: "off", Slug: "off", AppliesTo: []string{"llm"}, Active: false, Order: 2,
		Fields: []MetadataFieldDef{{Key: "a", Type: "string", Order: 1}}}
	require.NoError(t, inactive.Create(db))
	active := &MetadataSchema{Name: "on", Slug: "on", AppliesTo: []string{"*"}, Active: true, Order: 1}
	require.NoError(t, active.Create(db))

	var loaded MetadataSchema
	require.NoError(t, loaded.Get(db, inactive.ID))
	assert.False(t, loaded.Active)
	assert.Equal(t, "a", loaded.Fields[0].Key, "fields serialise as JSON")
	assert.Equal(t, "advisory", loaded.Enforcement, "enforcement defaults to advisory")
	assert.Equal(t, "admin", loaded.Source)

	var all MetadataSchemas
	require.NoError(t, all.GetAll(db, false))
	assert.Equal(t, []string{"on", "off"}, []string{all[0].Slug, all[1].Slug}, "ordered by order column")
	var activeOnly MetadataSchemas
	require.NoError(t, activeOnly.GetAll(db, true))
	assert.Len(t, activeOnly, 1)

	var bySlug MetadataSchema
	require.NoError(t, bySlug.GetBySlug(db, "on"))
	assert.Equal(t, active.ID, bySlug.ID)
	var missing MetadataSchema
	assert.ErrorIs(t, missing.GetBySlug(db, "missing"), gorm.ErrRecordNotFound)

	// Slug uniqueness is enforced at the database level too.
	dup := &MetadataSchema{Name: "dup", Slug: "on", AppliesTo: []string{"*"}}
	assert.Error(t, dup.Create(db))

	// Object metadata is unique per (type, id) and looked up by object.
	rec := &ObjectMetadata{ObjectType: "llm", ObjectID: "1", Values: JSONMap{"k": "v"}, ValidationStatus: "valid"}
	require.NoError(t, db.Create(rec).Error)
	assert.Error(t, db.Create(&ObjectMetadata{ObjectType: "llm", ObjectID: "1"}).Error)
	var got ObjectMetadata
	require.NoError(t, got.GetByObject(db, "llm", "1"))
	assert.Equal(t, "v", got.Values["k"])
	assert.ErrorIs(t, got.GetByObject(db, "llm", "2"), gorm.ErrRecordNotFound)

	vocab := &MetadataVocabulary{Name: "Risk", Slug: "risk", Terms: []VocabularyTerm{{Value: "low"}}}
	require.NoError(t, vocab.Create(db))
	var vs MetadataVocabularies
	require.NoError(t, vs.GetAll(db))
	assert.Len(t, vs, 1)
	assert.Equal(t, "low", vs[0].Terms[0].Value)
}
