package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// w4e: a plugin's before-hook cannot set a tool's or datasource's "Allow
// internal host" switch; the stored value wins.
func TestObjectHooks_CannotSetAllowInternalHost(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	plugin := &hookPlugin{modify: func(obj map[string]any) {
		obj["allow_internal_host"] = true
		if _, ok := obj["description"]; ok {
			obj["description"] = "set by the hook"
		}
	}}
	registry := NewHookRegistry()
	var regs []*pb.ObjectHookRegistration
	for _, ot := range []ObjectType{ObjectTypeTool, ObjectTypeDatasource} {
		regs = append(regs, &pb.ObjectHookRegistration{ObjectType: string(ot),
			HookTypes: []string{string(HookBeforeCreate), string(HookBeforeUpdate)}})
	}
	require.NoError(t, registry.RegisterHooks(1, "flipper", regs))
	manager := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{1: {ID: 1, Name: "flipper", GRPCClient: plugin, IsHealthy: true}}}
	s.HookManager = NewHookManager(registry, manager)

	tool, err := s.CreateTool("Weather", "d", models.ToolTypeREST, "", 10, "", "")
	require.NoError(t, err)
	stored, err := s.GetToolByID(tool.ID)
	require.NoError(t, err)
	assert.Equal(t, "set by the hook", stored.Description, "the hook's other changes apply")
	assert.False(t, stored.AllowInternalHost)

	ds, err := s.CreateDatasource("Docs", "s", "l", "", "", 10, 1, nil, "postgres://db/vec", "pgvector", "", "db",
		EmbedderInput{Vendor: "openai", URL: "https://embed.example", Model: "m"}, true)
	require.NoError(t, err)
	storedDS, err := s.GetDatasourceByID(ds.ID)
	require.NoError(t, err)
	assert.False(t, storedDS.AllowInternalHost)
}
