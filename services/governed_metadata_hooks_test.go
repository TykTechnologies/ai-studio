package services

import (
	"context"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewMetadataHookRunner_NilManagerMeansNoHooks(t *testing.T) {
	assert.Nil(t, newMetadataHookRunner(nil))
}

func TestMetadataHookRunner_NoRegisteredHooksAllows(t *testing.T) {
	hm := NewHookManager(NewHookRegistry(), nil)
	runner := newMetadataHookRunner(hm)
	require.NotNil(t, runner)
	rec := &models.ObjectMetadata{ObjectType: "llm", ObjectID: "1", Values: models.JSONMap{"k": "v"}}
	out, err := runner.RunMetadataHook(context.Background(), governed_metadata.HookBeforeUpdate, rec, 7)
	require.NoError(t, err)
	assert.True(t, out.Allowed)
	assert.Nil(t, out.Modified)
	assert.Empty(t, out.Executed)
}

func TestHookRegistry_AcceptsGovernedMetadataType(t *testing.T) {
	r := NewHookRegistry()
	assert.True(t, r.isValidObjectType(ObjectTypeGovernedMetadata))
	assert.False(t, r.isValidObjectType(ObjectType("spaceship")))
}

func TestHookOutcomeFromResult(t *testing.T) {
	hm := NewHookManager(NewHookRegistry(), nil)
	rec := &models.ObjectMetadata{ObjectType: "llm", ObjectID: "1", Values: models.JSONMap{"k": "v"}}

	t.Run("nil result allows", func(t *testing.T) {
		out := hookOutcomeFromResult(hm, nil, rec)
		assert.True(t, out.Allowed)
	})

	t.Run("rejection is passed through", func(t *testing.T) {
		out := hookOutcomeFromResult(hm, &HookExecutionResult{Allowed: false, RejectionReason: "no", ModifiedObject: rec, Executed: []string{"p1"}}, rec)
		assert.False(t, out.Allowed)
		assert.Equal(t, "no", out.RejectionReason)
		assert.Equal(t, []string{"p1"}, out.Executed)
		assert.Nil(t, out.Modified, "the pre-seeded original is not a modification")
	})

	t.Run("a different record is reported as modified", func(t *testing.T) {
		changed := &models.ObjectMetadata{ObjectType: "llm", ObjectID: "1", Values: models.JSONMap{"k": "changed"}}
		out := hookOutcomeFromResult(hm, &HookExecutionResult{Allowed: true, ModifiedObject: changed}, rec)
		assert.Same(t, changed, out.Modified)
	})

	t.Run("plugin metadata merges into a copy without mutating the input", func(t *testing.T) {
		out := hookOutcomeFromResult(hm, &HookExecutionResult{Allowed: true, ModifiedObject: rec, Metadata: map[string]string{"plugin_1_tag": "x"}}, rec)
		require.NotNil(t, out.Modified)
		assert.Equal(t, "x", out.Modified.Values["plugin_1_tag"])
		assert.Equal(t, "v", out.Modified.Values["k"])
		_, leaked := rec.Values["plugin_1_tag"]
		assert.False(t, leaked)
	})

	t.Run("a wrong object type in the result is ignored", func(t *testing.T) {
		out := hookOutcomeFromResult(hm, &HookExecutionResult{Allowed: true, ModifiedObject: &models.LLM{}}, rec)
		assert.Nil(t, out.Modified)
	})
}

func TestHookManager_UnmarshalAndMergeGovernedMetadata(t *testing.T) {
	hm := NewHookManager(NewHookRegistry(), nil)
	obj, err := hm.unmarshalObject(ObjectTypeGovernedMetadata, `{"object_type":"llm","object_id":"9","values":{"a":1}}`)
	require.NoError(t, err)
	rec, ok := obj.(*models.ObjectMetadata)
	require.True(t, ok)
	assert.Equal(t, "9", rec.ObjectID)
	assert.EqualValues(t, 1, rec.Values["a"])

	empty := &models.ObjectMetadata{}
	require.NoError(t, hm.MergeMetadata(empty, map[string]string{"x": "y"}))
	assert.Equal(t, "y", empty.Values["x"])

	_, err = hm.unmarshalObject(ObjectTypeGovernedMetadata, `not json`)
	assert.Error(t, err)
	_, err = hm.unmarshalObject(ObjectType("spaceship"), `{}`)
	assert.Error(t, err)
}

func TestServiceGovernedMetadataAccessorIsNilSafe(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// The enterprise service lists opted-in plugin resource types alongside the built-ins.
	require.NoError(t, db.AutoMigrate(&models.PluginResourceType{}))

	var nilService *Service
	assert.NotNil(t, nilService.GovernedMetadata(), "nil receiver still yields the edition default")

	svc := &Service{DB: db}
	first := svc.GovernedMetadata()
	require.NotNil(t, first)
	assert.Same(t, first, svc.GovernedMetadata(), "lazily created once and cached")

	// Sanity: the default service answers reads without a database write.
	types, err := first.ListObjectTypes()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(types), 3)
	_ = errors.New
}
