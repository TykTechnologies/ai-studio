package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// A plugin's before-hook cannot move an object to another Workspace (or out
// of one): the stored workspace_id wins, while the hook's other changes
// apply (midsommar-clean-oztk.2).
func TestObjectHooks_CannotChangeWorkspaceID(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	moveTo := float64(999)
	plugin := &hookPlugin{modify: func(obj map[string]any) {
		obj["workspace_id"] = moveTo
		for _, k := range []string{"description", "short_description"} {
			if _, ok := obj[k]; ok {
				obj[k] = "set by the hook"
			}
		}
	}}
	registry := NewHookRegistry()
	var regs []*pb.ObjectHookRegistration
	for _, ot := range []ObjectType{ObjectTypeLLM, ObjectTypeTool, ObjectTypeDatasource} {
		regs = append(regs, &pb.ObjectHookRegistration{ObjectType: string(ot),
			HookTypes: []string{string(HookBeforeCreate), string(HookBeforeUpdate)}})
	}
	require.NoError(t, registry.RegisterHooks(1, "mover", regs))
	manager := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{1: {ID: 1, Name: "mover", GRPCClient: plugin, IsHealthy: true}}}
	s.HookManager = NewHookManager(registry, manager)

	// Create: the hook's workspace_id is dropped.
	tool, err := s.CreateTool("Weather", "d", models.ToolTypeREST, "", 10, "", "")
	require.NoError(t, err)
	stored, err := s.GetToolByID(tool.ID)
	require.NoError(t, err)
	assert.Equal(t, "set by the hook", stored.Description, "the hook's other changes apply")
	assert.Zero(t, stored.WorkspaceID)

	llm, err := s.CreateLLM("Hooked", "", "https://api.example", 50, "", "", "", models.OPENAI, true,
		nil, "gpt", nil, nil, nil, false, nil, nil)
	require.NoError(t, err)
	storedLLM, err := s.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, "set by the hook", storedLLM.ShortDescription)
	assert.Zero(t, storedLLM.WorkspaceID)

	ds, err := s.CreateDatasource("Docs", "s", "l", "", "", 10, 1, nil, "postgres://db/vec", "pgvector", "", "db",
		EmbedderInput{Vendor: "openai", URL: "https://embed.example", Model: "m"}, true)
	require.NoError(t, err)
	storedDS, err := s.GetDatasourceByID(ds.ID)
	require.NoError(t, err)
	assert.Zero(t, storedDS.WorkspaceID)

	// Update: an object in a Workspace stays there, whether the hook names
	// another one or zeroes it.
	ws := &models.Workspace{Name: "Owned"}
	require.NoError(t, db.Create(ws).Error)
	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", tool.ID).UpdateColumn("workspace_id", ws.ID).Error)
	for _, to := range []float64{999, 0} {
		moveTo = to
		_, err = s.UpdateTool(tool.ID, "Weather", "d2", models.ToolTypeREST, "", 10, "", "")
		require.NoError(t, err)
		stored, err = s.GetToolByID(tool.ID)
		require.NoError(t, err)
		assert.Equal(t, ws.ID, stored.WorkspaceID, "hook set workspace_id %v", to)
	}
}

// A contributor's update submission (and a version rollback, which goes
// through the same map) cannot move the resource to another Workspace.
func TestSubmissionUpdatesIgnoreWorkspaceID(t *testing.T) {
	updates := payloadToUpdatesMap(models.JSONMap{"name": "x", "workspace_id": float64(7)})
	assert.NotContains(t, updates, "workspace_id")
	assert.Equal(t, "x", updates["name"])
}
