package services

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDependentsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	return db
}

func TestDependents_EmptyArraysAreAlwaysPresent(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	llm := &models.LLM{Name: "Lonely", Active: true}
	require.NoError(t, db.Create(llm).Error)

	deps, err := s.GetLLMDependents(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, deps.Total)

	raw, err := json.Marshal(deps)
	require.NoError(t, err)
	for _, key := range []string{"apps", "catalogues", "llms", "tools", "datasources", "agents", "model_routers", "chats"} {
		assert.Contains(t, string(raw), `"`+key+`":[]`, "every array is present and empty, never null")
	}
	assert.Contains(t, string(raw), `"total":0`)
}

func TestGetLLMDependents(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	llm := &models.LLM{Name: "Primary", Active: true}
	require.NoError(t, db.Create(llm).Error)
	unrelated := &models.LLM{Name: "Unrelated", Active: true}
	require.NoError(t, db.Create(unrelated).Error)

	app := &models.App{Name: "Billing Copilot", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Model(app).Association("LLMs").Append(llm))
	otherApp := &models.App{Name: "Other App", UserID: 1}
	require.NoError(t, db.Create(otherApp).Error)
	require.NoError(t, db.Model(otherApp).Association("LLMs").Append(unrelated))

	catalogue := &models.Catalogue{Name: "Platform LLMs"}
	require.NoError(t, db.Create(catalogue).Error)
	require.NoError(t, db.Model(catalogue).Association("LLMs").Append(llm))

	agent := &models.AgentConfig{Name: "Triage", Slug: "triage", PluginID: 1, AppID: app.ID}
	require.NoError(t, db.Create(agent).Error)

	backup := &models.LLM{Name: "Backup Primary", Active: true,
		Failover: models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: llm.ID, Model: "gpt-4"}}}}
	require.NoError(t, db.Create(backup).Error)

	// The same LLM in two pools of one router must still yield one router.
	router := &models.ModelRouter{Name: "Edge Router", Slug: "edge", Pools: []*models.ModelPool{
		{Name: "pool-a", ModelPattern: "gpt-*", Vendors: []*models.PoolVendor{{LLMID: llm.ID}}},
		{Name: "pool-b", ModelPattern: "claude-*", Vendors: []*models.PoolVendor{{LLMID: llm.ID}}},
	}}
	require.NoError(t, db.Create(router).Error)

	chat := &models.Chat{Name: "Support Chat", LLMID: llm.ID}
	require.NoError(t, db.Create(chat).Error)

	deps, err := s.GetLLMDependents(llm.ID)
	require.NoError(t, err)

	assert.Equal(t, []DependentRef{{ID: app.ID, Name: "Billing Copilot"}}, deps.Apps)
	assert.Equal(t, []DependentRef{{ID: catalogue.ID, Name: "Platform LLMs"}}, deps.Catalogues)
	assert.Equal(t, []DependentRef{{ID: agent.ID, Name: "Triage"}}, deps.Agents)
	assert.Equal(t, []DependentRef{{ID: backup.ID, Name: "Backup Primary"}}, deps.LLMs)
	assert.Equal(t, []DependentRef{{ID: router.ID, Name: "Edge Router"}}, deps.ModelRouters)
	assert.Equal(t, []DependentRef{{ID: chat.ID, Name: "Support Chat"}}, deps.Chats)
	assert.Empty(t, deps.Tools)
	assert.Empty(t, deps.Datasources)
	assert.Equal(t, 6, deps.Total)

	// Soft-deleted apps are not dependents.
	require.NoError(t, db.Delete(app).Error)
	deps, err = s.GetLLMDependents(llm.ID)
	require.NoError(t, err)
	assert.Empty(t, deps.Apps)
	assert.Equal(t, 5, deps.Total)
}

func TestGetToolDependents(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	tool := &models.Tool{Name: "Weather", Active: true}
	require.NoError(t, db.Create(tool).Error)

	app := &models.App{Name: "Forecast App", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Model(app).Association("Tools").Append(tool))

	catalogue := &models.ToolCatalogue{Name: "Public Tools"}
	require.NoError(t, db.Create(catalogue).Error)
	require.NoError(t, db.Model(catalogue).Association("Tools").Append(tool))

	consumer := &models.Tool{Name: "Trip Planner", Active: true}
	require.NoError(t, db.Create(consumer).Error)
	require.NoError(t, consumer.AddDependency(db, tool))

	agent := &models.AgentConfig{Name: "Planner Agent", Slug: "planner", PluginID: 1, AppID: app.ID}
	require.NoError(t, db.Create(agent).Error)

	chat := &models.Chat{Name: "Travel Chat", LLMID: 1}
	require.NoError(t, db.Create(chat).Error)
	require.NoError(t, db.Model(chat).Association("DefaultTools").Append(tool))

	deps, err := s.GetToolDependents(tool.ID)
	require.NoError(t, err)

	assert.Equal(t, []DependentRef{{ID: app.ID, Name: "Forecast App"}}, deps.Apps)
	assert.Equal(t, []DependentRef{{ID: catalogue.ID, Name: "Public Tools"}}, deps.Catalogues)
	assert.Equal(t, []DependentRef{{ID: consumer.ID, Name: "Trip Planner"}}, deps.Tools)
	assert.Equal(t, []DependentRef{{ID: agent.ID, Name: "Planner Agent"}}, deps.Agents)
	assert.Equal(t, []DependentRef{{ID: chat.ID, Name: "Travel Chat"}}, deps.Chats)
	assert.Equal(t, 5, deps.Total)

	// The dependency direction matters: the tool depended upon has no
	// dependents of its own from this edge.
	consumerDeps, err := s.GetToolDependents(consumer.ID)
	require.NoError(t, err)
	assert.Empty(t, consumerDeps.Tools)
}

func TestGetDatasourceDependents(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	ds := &models.Datasource{Name: "Docs", Active: true}
	require.NoError(t, db.Create(ds).Error)

	app := &models.App{Name: "Docs App", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Model(app).Association("Datasources").Append(ds))

	catalogue := &models.DataCatalogue{Name: "Knowledge"}
	require.NoError(t, db.Create(catalogue).Error)
	require.NoError(t, db.Model(catalogue).Association("Datasources").Append(ds))

	agent := &models.AgentConfig{Name: "Docs Agent", Slug: "docs", PluginID: 1, AppID: app.ID}
	require.NoError(t, db.Create(agent).Error)

	chat := &models.Chat{Name: "Docs Chat", LLMID: 1, DefaultDataSourceID: &ds.ID}
	require.NoError(t, db.Create(chat).Error)

	deps, err := s.GetDatasourceDependents(ds.ID)
	require.NoError(t, err)

	assert.Equal(t, []DependentRef{{ID: app.ID, Name: "Docs App"}}, deps.Apps)
	assert.Equal(t, []DependentRef{{ID: catalogue.ID, Name: "Knowledge"}}, deps.Catalogues)
	assert.Equal(t, []DependentRef{{ID: agent.ID, Name: "Docs Agent"}}, deps.Agents)
	assert.Equal(t, []DependentRef{{ID: chat.ID, Name: "Docs Chat"}}, deps.Chats)
	assert.Equal(t, 4, deps.Total)
}

func TestGetFilterDependents(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	filter := &models.Filter{Name: "PII Scrub"}
	require.NoError(t, db.Create(filter).Error)

	llm := &models.LLM{Name: "Filtered LLM", Active: true}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, db.Model(llm).Association("Filters").Append(filter))

	tool := &models.Tool{Name: "Filtered Tool", Active: true}
	require.NoError(t, db.Create(tool).Error)
	require.NoError(t, db.Model(tool).Association("Filters").Append(filter))

	chat := &models.Chat{Name: "Filtered Chat", LLMID: llm.ID}
	require.NoError(t, db.Create(chat).Error)
	require.NoError(t, db.Model(chat).Association("Filters").Append(filter))

	deps, err := s.GetFilterDependents(filter.ID)
	require.NoError(t, err)

	assert.Equal(t, []DependentRef{{ID: llm.ID, Name: "Filtered LLM"}}, deps.LLMs)
	assert.Equal(t, []DependentRef{{ID: tool.ID, Name: "Filtered Tool"}}, deps.Tools)
	assert.Equal(t, []DependentRef{{ID: chat.ID, Name: "Filtered Chat"}}, deps.Chats)
	assert.Equal(t, 3, deps.Total)
}

func TestGetModelRouterDependents_IsEmpty(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	deps, err := s.GetModelRouterDependents(1)
	require.NoError(t, err)
	assert.Equal(t, 0, deps.Total)
	assert.NotNil(t, deps.Apps)
}

func TestSecretReferences(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	// api_key and api_endpoint both point at OPENAI_KEY: one reference, not two.
	openai := &models.LLM{Name: "OpenAI Prod", APIKey: "$SECRET/OPENAI_KEY", APIEndpoint: "$SECRET/OPENAI_KEY", Active: true}
	require.NoError(t, db.Create(openai).Error)
	bedrock := &models.LLM{Name: "Bedrock", APIKey: "", Active: true,
		Metadata: models.JSONMap{"aws_access_key_id": "AKIA...", "aws_secret_access_key": "$SECRET/AWS_SECRET"}}
	require.NoError(t, db.Create(bedrock).Error)
	inline := &models.LLM{Name: "Inline", APIKey: "sk-literal", Active: true}
	require.NoError(t, db.Create(inline).Error)
	env := &models.LLM{Name: "Env", APIKey: "$ENV/OPENAI_API_KEY", Active: true}
	require.NoError(t, db.Create(env).Error)

	tool := &models.Tool{Name: "CRM", AuthKey: "$SECRET/CRM_TOKEN", Active: true}
	require.NoError(t, db.Create(tool).Error)

	ds := &models.Datasource{Name: "Vectors", EmbedAPIKey: "$SECRET/OPENAI_KEY", DBConnAPIKey: "$SECRET/PGVECTOR", Active: true}
	require.NoError(t, db.Create(ds).Error)

	refs, err := s.SecretReferences()
	require.NoError(t, err)

	assert.Equal(t, []SecretReference{
		{Type: "datasource", ID: ds.ID, Name: "Vectors"},
		{Type: "llm", ID: openai.ID, Name: "OpenAI Prod"},
	}, refs["OPENAI_KEY"])
	assert.Equal(t, []SecretReference{{Type: "llm", ID: bedrock.ID, Name: "Bedrock"}}, refs["AWS_SECRET"])
	assert.Equal(t, []SecretReference{{Type: "tool", ID: tool.ID, Name: "CRM"}}, refs["CRM_TOKEN"])
	assert.Equal(t, []SecretReference{{Type: "datasource", ID: ds.ID, Name: "Vectors"}}, refs["PGVECTOR"])
	assert.NotContains(t, refs, "OPENAI_API_KEY", "$ENV/ references are not secrets")
	assert.Len(t, refs, 4)

	deps, err := s.GetSecretDependents("OPENAI_KEY")
	require.NoError(t, err)
	assert.Equal(t, []DependentRef{{ID: openai.ID, Name: "OpenAI Prod"}}, deps.LLMs)
	assert.Equal(t, []DependentRef{{ID: ds.ID, Name: "Vectors"}}, deps.Datasources)
	assert.Empty(t, deps.Tools)
	assert.Equal(t, 2, deps.Total)

	none, err := s.GetSecretDependents("UNUSED")
	require.NoError(t, err)
	assert.Equal(t, 0, none.Total)
}

// A secret whose name contains LIKE wildcards must match only itself: the
// per-secret lookup escapes the name, so OPENAI_KEY does not pick up
// OPENAIXKEY, and metadata references are found by the quoted reference.
func TestGetSecretDependents_TargetedMatching(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	wanted := &models.LLM{Name: "Wanted", APIKey: "$SECRET/OPENAI_KEY", Active: true}
	require.NoError(t, db.Create(wanted).Error)
	decoy := &models.LLM{Name: "Decoy", APIKey: "$SECRET/OPENAIXKEY", Active: true}
	require.NoError(t, db.Create(decoy).Error)
	longer := &models.LLM{Name: "Longer", APIKey: "$SECRET/OPENAI_KEY_2", Active: true}
	require.NoError(t, db.Create(longer).Error)
	viaMetadata := &models.LLM{Name: "Bedrock", Active: true,
		Metadata: models.JSONMap{"aws_secret_access_key": "$SECRET/OPENAI_KEY", "note": "mentions $SECRET/OPENAI_KEY_2 in text"}}
	require.NoError(t, db.Create(viaMetadata).Error)
	tool := &models.Tool{Name: "CRM", AuthKey: "$SECRET/OPENAI_KEY", Active: true}
	require.NoError(t, db.Create(tool).Error)

	deps, err := s.GetSecretDependents("OPENAI_KEY")
	require.NoError(t, err)
	assert.Equal(t, []DependentRef{{ID: wanted.ID, Name: "Wanted"}, {ID: viaMetadata.ID, Name: "Bedrock"}}, deps.LLMs)
	assert.Equal(t, []DependentRef{{ID: tool.ID, Name: "CRM"}}, deps.Tools)
	assert.Equal(t, 3, deps.Total)

	deps2, err := s.GetSecretDependents("OPENAI_KEY_2")
	require.NoError(t, err)
	assert.Equal(t, []DependentRef{{ID: longer.ID, Name: "Longer"}}, deps2.LLMs,
		"a quoted mention inside free text is not a reference; only whole-value references count")

	all, err := s.SecretReferences()
	require.NoError(t, err)
	assert.Len(t, all["OPENAI_KEY"], 3)
	assert.Len(t, all["OPENAIXKEY"], 1)
	assert.Len(t, all["OPENAI_KEY_2"], 1)
}

// The portal's tool-catalogue listing filters inactive tools in the database;
// an unknown catalogue is still an error rather than an empty list.
func TestGetToolCatalogueActiveTools(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	catalogue, err := s.CreateToolCatalogue("Ops tools", "", "", "")
	require.NoError(t, err)

	live := &models.Tool{Name: "Live", Active: true}
	require.NoError(t, db.Create(live).Error)
	draft := &models.Tool{Name: "Draft"}
	require.NoError(t, db.Create(draft).Error)
	// gorm:"default:true" turns an explicit false into true on insert, so
	// deactivate with an update, as the activate/deactivate endpoints do.
	require.NoError(t, db.Model(draft).Update("active", false).Error)
	require.NoError(t, catalogue.AddTool(db, live))
	require.NoError(t, catalogue.AddTool(db, draft))

	all, err := s.GetToolCatalogueTools(catalogue.ID)
	require.NoError(t, err)
	assert.Len(t, all, 2, "administrators still see the draft")

	active, err := s.GetToolCatalogueActiveTools(catalogue.ID)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "Live", active[0].Name)

	_, err = s.GetToolCatalogueActiveTools(catalogue.ID + 100)
	assert.Error(t, err)
}

// The secret reference index follows every write path: create, a full
// update, a partial update through a loaded model, the metadata-only update,
// delete, and a startup backfill over rows written behind the hooks' back.
func TestSecretReferenceIndex_FollowsWrites(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	llm := &models.LLM{Name: "Prod", APIKey: "$SECRET/OPENAI_KEY", Active: true}
	require.NoError(t, db.Create(llm).Error)
	refs, err := s.SecretReferences()
	require.NoError(t, err)
	assert.Len(t, refs["OPENAI_KEY"], 1, "create indexes the reference")

	// Full update: the reference moves to the new secret.
	llm.APIKey = "$SECRET/OPENAI_KEY_V2"
	require.NoError(t, db.Save(llm).Error)
	refs, err = s.SecretReferences()
	require.NoError(t, err)
	assert.Empty(t, refs["OPENAI_KEY"])
	assert.Len(t, refs["OPENAI_KEY_V2"], 1)

	// Partial update of an unrelated column keeps the reference (the hook
	// re-reads the row rather than trusting the partial struct).
	require.NoError(t, db.Model(llm).Update("active", false).Error)
	refs, err = s.SecretReferences()
	require.NoError(t, err)
	assert.Len(t, refs["OPENAI_KEY_V2"], 1)

	// Metadata-only update through the service adds a second secret.
	require.NoError(t, s.UpdateLLMMetadata(llm.ID, models.JSONMap{"aws_secret_access_key": "$SECRET/AWS_SECRET"}))
	refs, err = s.SecretReferences()
	require.NoError(t, err)
	assert.Len(t, refs["AWS_SECRET"], 1)
	assert.Len(t, refs["OPENAI_KEY_V2"], 1)

	// Soft delete clears both.
	require.NoError(t, db.Delete(llm).Error)
	refs, err = s.SecretReferences()
	require.NoError(t, err)
	assert.Empty(t, refs)

	// A row written without hooks (raw SQL) is picked up by the backfill.
	require.NoError(t, db.Exec("INSERT INTO tools (name, slug, auth_key, active, created_at, updated_at) VALUES ('Raw', 'raw', '$SECRET/CRM_TOKEN', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Error)
	refs, err = s.SecretReferences()
	require.NoError(t, err)
	assert.Empty(t, refs["CRM_TOKEN"], "raw inserts bypass the hooks")
	require.NoError(t, models.BackfillSecretReferences(db))
	refs, err = s.SecretReferences()
	require.NoError(t, err)
	assert.Len(t, refs["CRM_TOKEN"], 1, "the backfill repairs the index")
}

// The secrets list asks only for the names on its page, so the lookup is
// bounded by the page rather than the whole index.
func TestSecretReferencesByNames_IsScopedToTheRequestedNames(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)

	require.NoError(t, db.Create(&models.LLM{Name: "A", APIKey: "$SECRET/ALPHA", Active: true}).Error)
	require.NoError(t, db.Create(&models.LLM{Name: "B", APIKey: "$SECRET/BETA", Active: true}).Error)
	require.NoError(t, db.Create(&models.Tool{Name: "C", AuthKey: "$SECRET/GAMMA", Active: true}).Error)

	page, err := s.SecretReferencesByNames([]string{"ALPHA", "GAMMA", "UNUSED"})
	require.NoError(t, err)
	assert.Len(t, page, 2, "only the requested names that have consumers are returned")
	assert.Len(t, page["ALPHA"], 1)
	assert.Len(t, page["GAMMA"], 1)
	assert.NotContains(t, page, "BETA")

	empty, err := s.SecretReferencesByNames(nil)
	require.NoError(t, err)
	assert.Empty(t, empty, "no names means no lookup, not every secret")
}
