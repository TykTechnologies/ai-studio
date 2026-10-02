package grpc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

// Every governance read and team-grant RPC refuses a plugin without its scope.
func TestGovernanceRPCs_RequireScopes(t *testing.T) {
	server, _, _, ctx := setupExtensionRPCTest(t) // no scopes

	calls := map[string]func() error{
		"ListObjectMetadataAudit": func() error {
			_, err := server.ListObjectMetadataAudit(ctx, &pb.ListObjectMetadataAuditRequest{ObjectType: "llm", ObjectId: "1"})
			return err
		},
		"ListAuditRecords": func() error {
			_, err := server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "llm", ResourceId: "1"})
			return err
		},
		"ListMCPServers":      func() error { _, err := server.ListMCPServers(ctx, &pb.ListMCPServersRequest{}); return err },
		"GetMCPServer":        func() error { _, err := server.GetMCPServer(ctx, &pb.GetMCPServerRequest{ServerId: 1}); return err },
		"ListModelRouters":    func() error { _, err := server.ListModelRouters(ctx, &pb.ListModelRoutersRequest{}); return err },
		"GetModelRouter":      func() error { _, err := server.GetModelRouter(ctx, &pb.GetModelRouterRequest{RouterId: 1}); return err },
		"ListSemanticRouters": func() error { _, err := server.ListSemanticRouters(ctx, &pb.ListSemanticRoutersRequest{}); return err },
		"GetSemanticRouter": func() error {
			_, err := server.GetSemanticRouter(ctx, &pb.GetSemanticRouterRequest{RouterId: 1})
			return err
		},
		"ListGroups": func() error { _, err := server.ListGroups(ctx, &pb.ListGroupsRequest{}); return err },
		"GetResourceInstanceGroups": func() error {
			_, err := server.GetResourceInstanceGroups(ctx, &pb.GetResourceInstanceGroupsRequest{ResourceTypeSlug: "agent", InstanceId: "x"})
			return err
		},
		"SetResourceInstanceGroups": func() error {
			_, err := server.SetResourceInstanceGroups(ctx, &pb.SetResourceInstanceGroupsRequest{ResourceTypeSlug: "agent", InstanceId: "x"})
			return err
		},
		"SetAppGovernanceState": func() error {
			_, err := server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: 1})
			return err
		},
		"ListAccessibleResourceInstances": func() error {
			_, err := server.ListAccessibleResourceInstances(ctx, &pb.ListAccessibleResourceInstancesRequest{ResourceTypeSlug: "agent", UserId: 1})
			return err
		},
	}
	for name, call := range calls {
		assert.Equal(t, codes.PermissionDenied, status.Code(call()), name)
		assert.NotEmpty(t, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/"+name), "%s needs a scope mapping for the interceptor", name)
	}
}

func TestMCPServerRPCs_NeverCarryUpstreamOrAuth(t *testing.T) {
	server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeMCPServersRead)
	score := 70
	srv := &models.MCPServer{
		Name: "Billing MCP", Slug: "billing", Description: "Invoices", Kind: "remote",
		UpstreamURL: "https://secret-upstream.internal/mcp", AuthDetailsJSON: `{"token":"s3cret"}`,
		Definition: `{"upstream":"secret-upstream"}`, PrivacyScore: &score, IsActive: true,
	}
	require.NoError(t, service.GetDB().Create(srv).Error)
	require.NoError(t, service.GetDB().Create(&models.MCPServer{Name: "Inactive", Slug: "inactive", Kind: "remote"}).Error)

	got, err := server.GetMCPServer(ctx, &pb.GetMCPServerRequest{ServerId: uint32(srv.ID)})
	require.NoError(t, err)
	assert.Equal(t, "Billing MCP", got.Server.Name)
	require.NotNil(t, got.Server.PrivacyScore)
	assert.EqualValues(t, 70, *got.Server.PrivacyScore)
	wire, err := protojson.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "secret-upstream")
	assert.NotContains(t, string(wire), "s3cret")

	list, err := server.ListMCPServers(ctx, &pb.ListMCPServersRequest{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, list.TotalCount)
	active := true
	list, err = server.ListMCPServers(ctx, &pb.ListMCPServersRequest{IsActive: &active})
	require.NoError(t, err)
	require.Len(t, list.Servers, 1)
	assert.Equal(t, "billing", list.Servers[0].Slug)

	_, err = server.GetMCPServer(ctx, &pb.GetMCPServerRequest{ServerId: 9999})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestSemanticRouterTargets(t *testing.T) {
	r := &models.SemanticRouter{
		Routes: []semanticrouting.Route{
			{Name: "code", Target: semanticrouting.Target{Type: semanticrouting.TargetLLM, LLMID: 3}},
			{Name: "cheap", Target: semanticrouting.Target{Type: semanticrouting.TargetModelRouter, ModelRouterID: 7}},
			{Name: "dup", Target: semanticrouting.Target{Type: semanticrouting.TargetLLM, LLMID: 3}},
		},
	}
	r.Settings.Judge.Enabled = true
	r.Settings.Judge.ModelRef.LLMID = 1
	llms, routers := semanticRouterTargets(r)
	assert.Equal(t, []uint32{1, 3}, llms)
	assert.Equal(t, []uint32{7}, routers)

	r.Settings.Judge.Enabled = false
	llms, _ = semanticRouterTargets(r)
	assert.Equal(t, []uint32{3}, llms, "a disabled judge is not a target")
}

func TestModelRouterRPCs_ResolveTargetLLMs(t *testing.T) {
	server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeRoutersRead)
	db := service.GetDB()
	router := &models.ModelRouter{Name: "Router", Slug: "router"}
	require.NoError(t, db.Create(router).Error)
	pool := &models.ModelPool{RouterID: router.ID, Name: "pool", ModelPattern: "*"}
	require.NoError(t, db.Create(pool).Error)
	for _, llmID := range []uint{5, 2, 5} {
		require.NoError(t, db.Create(&models.PoolVendor{PoolID: pool.ID, LLMID: llmID}).Error)
	}

	got, err := modelRouterLLMIDs(db, []uint{router.ID})
	require.NoError(t, err)
	assert.Equal(t, []uint32{2, 5}, got[router.ID])

	list, err := server.ListModelRouters(ctx, &pb.ListModelRoutersRequest{})
	if !model_router.IsEnterpriseAvailable() {
		assert.Equal(t, codes.Unimplemented, status.Code(err), "CE: routers are an Enterprise feature")
		return
	}
	require.NoError(t, err)
	require.Len(t, list.Routers, 1)
	assert.Equal(t, []uint32{2, 5}, list.Routers[0].LlmIds)

	one, err := server.GetModelRouter(ctx, &pb.GetModelRouterRequest{RouterId: uint32(router.ID)})
	require.NoError(t, err)
	assert.Equal(t, []uint32{2, 5}, one.Router.LlmIds)
}

func TestSemanticRouterRPCs_FollowEdition(t *testing.T) {
	server, service, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeRoutersRead)
	r := &models.SemanticRouter{Name: "Semantic", Slug: "semantic", Routes: []semanticrouting.Route{
		{Name: "a", Target: semanticrouting.Target{Type: semanticrouting.TargetLLM, LLMID: 4}},
	}}
	require.NoError(t, service.GetDB().Create(r).Error)

	list, err := server.ListSemanticRouters(ctx, &pb.ListSemanticRoutersRequest{})
	if !semantic_router.IsEnterpriseAvailable() {
		assert.Equal(t, codes.Unimplemented, status.Code(err))
		return
	}
	require.NoError(t, err)
	require.Len(t, list.Routers, 1)
	assert.Equal(t, []uint32{4}, list.Routers[0].LlmIds)
}

func TestGetApp_CarriesAllBindings(t *testing.T) {
	server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeAppsRead, models.ServiceScopeResourceTypesManage)
	db := service.GetDB()

	app := &models.App{Name: "Bound App", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Exec("INSERT INTO app_mcp_servers (app_id, mcp_server_id) VALUES (?, ?)", app.ID, 11).Error)
	require.NoError(t, db.Exec("INSERT INTO app_model_routers (app_id, model_router_id) VALUES (?, ?)", app.ID, 12).Error)
	require.NoError(t, db.Exec("INSERT INTO app_semantic_routers (app_id, semantic_router_id) VALUES (?, ?)", app.ID, 13).Error)
	_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{Types: []*pb.ResourceTypeSpec{{Slug: "agent", Name: "Agent"}}})
	require.NoError(t, err)
	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.AppPluginResource{AppID: app.ID, PluginResourceTypeID: prt.ID, InstanceID: "ast_1"}).Error)

	got, err := server.GetApp(ctx, &pb.GetAppRequest{AppId: uint32(app.ID)})
	require.NoError(t, err)
	assert.Equal(t, []uint32{11}, got.App.McpServerIds)
	assert.Equal(t, []uint32{12}, got.App.ModelRouterIds)
	assert.Equal(t, []uint32{13}, got.App.SemanticRouterIds)
	require.Len(t, got.App.PluginResources, 1)
	assert.Equal(t, uint32(plugin.ID), got.App.PluginResources[0].PluginId)
	assert.Equal(t, "agent", got.App.PluginResources[0].ResourceTypeSlug)
	assert.Equal(t, "ast_1", got.App.PluginResources[0].InstanceId)

	list, err := server.ListApps(ctx, &pb.ListAppsRequest{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, list.Apps, 1)
	assert.Equal(t, []uint32{11}, list.Apps[0].McpServerIds)
}

func TestListObjectMetadataAudit_ResolvesSelfType(t *testing.T) {
	server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeMetadataRead)
	objectType := models.PluginResourceObjectType(plugin.ID, "agent")
	for i, action := range []string{"set", "merge"} {
		row := &models.ObjectMetadataAudit{ObjectType: objectType, ObjectID: "ast_1", Action: action, Source: "plugin:1",
			After: models.JSONMap{"risk": action}}
		row.CreatedAt = time.Now().Add(time.Duration(i) * time.Minute)
		require.NoError(t, service.GetDB().Create(row).Error)
	}

	got, err := server.ListObjectMetadataAudit(ctx, &pb.ListObjectMetadataAuditRequest{ObjectType: "plugin_resource:self:agent", ObjectId: "ast_1"})
	require.NoError(t, err)
	if !governed_metadata.IsEnterpriseAvailable() {
		assert.Empty(t, got.Entries, "CE: no governed metadata history")
		return
	}
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "merge", got.Entries[0].Action, "newest first")
	assert.JSONEq(t, `{"risk":"merge"}`, got.Entries[0].AfterJson)

	_, err = server.ListObjectMetadataAudit(ctx, &pb.ListObjectMetadataAuditRequest{ObjectType: "llm"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListAuditRecords_FollowsEdition(t *testing.T) {
	server, service, _, ctx := setupExtensionRPCTestDSN(t, "file:"+t.Name()+"?mode=memory&cache=shared", models.ServiceScopeAuditRead)
	_, err := server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "llm"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// No audit service attached (proxy-only mode): unavailable, not a panic.
	_, err = server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "llm", ResourceId: "1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	svc := audit.NewService(service.GetDB(), config.AuditConfig{Enabled: true, StoreType: "db"})
	t.Cleanup(svc.Stop)
	service.SetAuditService(svc)
	if !audit.IsEnterpriseAvailable() {
		_, err = server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "llm", ResourceId: "1"})
		assert.Equal(t, codes.Unimplemented, status.Code(err), "CE: the audit trail is an Enterprise feature")
		return
	}

	now := time.Now()
	for i, rec := range []models.AuditRecord{
		{Action: "View LLM", Method: "GET", ResourceType: "llm", ResourceID: "1"},
		{Action: "Update LLM", Method: "PATCH", ResourceType: "llm", ResourceID: "1", Diff: models.RawJSON(`{"name":{"old":"a","new":"b"}}`),
			RequestDump: models.RawJSON(`{"secret":"body"}`)},
		{Action: "Delete LLM", Method: "DELETE", ResourceType: "llm", ResourceID: "1"},
		{Action: "Update LLM", Method: "PATCH", ResourceType: "llm", ResourceID: "2"},
	} {
		rec.Timestamp = now.Add(time.Duration(i) * time.Second)
		require.NoError(t, service.GetDB().Create(&rec).Error)
	}

	got, err := server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "llm", ResourceId: "1", MutationsOnly: true})
	require.NoError(t, err)
	require.Len(t, got.Records, 2, "reads are dropped, other resources are not included")
	assert.Equal(t, "DELETE", got.Records[0].Method, "newest first")
	assert.JSONEq(t, `{"name":{"old":"a","new":"b"}}`, got.Records[1].DiffJson)
	wire, err := protojson.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "secret", "request bodies are never returned")

	all, err := server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "llm", ResourceId: "1"})
	require.NoError(t, err)
	assert.Len(t, all.Records, 3)
}

func TestResourceInstanceGroupRPCs(t *testing.T) {
	server, service, plugin, ctx := setupExtensionRPCTest(t, models.ServiceScopeResourceAccessManage, models.ServiceScopeResourceTypesManage)
	db := service.GetDB()
	_, err := server.RegisterResourceTypes(ctx, &pb.RegisterResourceTypesRequest{Types: []*pb.ResourceTypeSpec{
		{Slug: "agent", Name: "Agent", DefaultAccess: models.DefaultAccessExplicit},
	}})
	require.NoError(t, err)
	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)
	assert.Equal(t, models.DefaultAccessExplicit, prt.DefaultAccess, "RegisterResourceTypes carries default_access")

	team := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(team).Error)
	member := &models.User{Email: "m@example.com", Name: "M"}
	require.NoError(t, db.Create(member).Error)
	require.NoError(t, service.AddUserToGroup(member.ID, team.ID))

	groups, err := server.ListGroups(ctx, &pb.ListGroupsRequest{})
	require.NoError(t, err)
	var names []string
	for _, g := range groups.Groups {
		names = append(names, g.Name)
	}
	assert.Contains(t, names, "Platform")

	set, err := server.SetResourceInstanceGroups(ctx, &pb.SetResourceInstanceGroupsRequest{
		ResourceTypeSlug: "agent", InstanceId: "ast_1", GroupIds: []uint32{uint32(team.ID)},
	})
	require.NoError(t, err)
	assert.Equal(t, []uint32{uint32(team.ID)}, set.GroupIds)

	read, err := server.GetResourceInstanceGroups(ctx, &pb.GetResourceInstanceGroupsRequest{ResourceTypeSlug: "agent", InstanceId: "ast_1"})
	require.NoError(t, err)
	assert.Equal(t, []uint32{uint32(team.ID)}, read.GroupIds)

	acc, err := server.ListAccessibleResourceInstances(ctx, &pb.ListAccessibleResourceInstancesRequest{ResourceTypeSlug: "agent", UserId: uint32(member.ID)})
	require.NoError(t, err)
	assert.False(t, acc.SeeAll)
	assert.Equal(t, []string{"ast_1"}, acc.InstanceIds)

	// Another plugin's type and unknown groups are refused.
	other := &models.Plugin{Name: "other", Command: "/x", HookType: models.HookTypeResourceProvider, IsActive: true}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, service.RegisterPluginResourceTypes(other.ID, []models.PluginResourceType{{Slug: "secret", Name: "Secret"}}))
	_, err = server.SetResourceInstanceGroups(ctx, &pb.SetResourceInstanceGroupsRequest{ResourceTypeSlug: "secret", InstanceId: "x", GroupIds: []uint32{uint32(team.ID)}})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = server.SetResourceInstanceGroups(ctx, &pb.SetResourceInstanceGroupsRequest{ResourceTypeSlug: "agent", InstanceId: "ast_1", GroupIds: []uint32{99999}})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = server.ListAccessibleResourceInstances(context.Background(), &pb.ListAccessibleResourceInstancesRequest{ResourceTypeSlug: "agent", UserId: 1})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestSetAppGovernanceState(t *testing.T) {
	server, service, _, ctx := setupExtensionRPCTestDSN(t, "file:"+t.Name()+"?mode=memory&cache=shared", models.ServiceScopeAppsLifecycle, models.ServiceScopeAuditRead)
	db := service.GetDB()
	app := &models.App{Name: "Support bot", UserID: 1, IsActive: true, Metadata: map[string]interface{}{"owner_note": "keep"}}
	require.NoError(t, db.Create(app).Error)
	auditSvc := audit.NewService(db, config.AuditConfig{Enabled: true, StoreType: "db"})
	t.Cleanup(auditSvc.Stop)
	service.SetAuditService(auditSvc)

	inactive := false
	resp, err := server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{
		AppId: uint32(app.ID), IsActive: &inactive, Flags: map[string]string{"review_lapsed": "2026-09-01"}, Reason: "review lapsed",
	})
	require.NoError(t, err)
	assert.True(t, resp.Changed)
	assert.False(t, resp.App.IsActive)

	var stored models.App
	require.NoError(t, db.First(&stored, app.ID).Error)
	assert.False(t, stored.IsActive)
	assert.Equal(t, "keep", stored.Metadata["owner_note"], "other metadata is untouched")
	flags := stored.Metadata[services.AppGovernanceFlagsKey].(map[string]interface{})
	flag := flags["review_lapsed"].(map[string]interface{})
	assert.Equal(t, "2026-09-01", flag["value"])
	assert.Equal(t, "review lapsed", flag["reason"])
	assert.Contains(t, flag["set_by"], "plugin:")

	again, err := server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: uint32(app.ID), IsActive: &inactive, Flags: map[string]string{"review_lapsed": "2026-09-01"}})
	require.NoError(t, err)
	assert.False(t, again.Changed, "idempotent")

	active := true
	_, err = server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: uint32(app.ID), IsActive: &active, Flags: map[string]string{"review_lapsed": ""}, Reason: "reviewed"})
	require.NoError(t, err)
	stored = models.App{}
	require.NoError(t, db.First(&stored, app.ID).Error)
	assert.True(t, stored.IsActive)
	_, hasFlags := stored.Metadata[services.AppGovernanceFlagsKey]
	assert.False(t, hasFlags, "the last flag cleared removes the key")

	_, err = server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: 9999})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: uint32(app.ID), Flags: map[string]string{"bad name": "x"}})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	tooMany := map[string]string{}
	for i := 0; i <= maxAppGovernanceFlags; i++ {
		tooMany[fmt.Sprintf("flag_%d", i)] = "x"
	}
	_, err = server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: uint32(app.ID), Flags: tooMany})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "at most 16 flags per call")
	_, err = server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: uint32(app.ID), Flags: map[string]string{strings.Repeat("n", 65): "x"}})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "flag names are capped at 64 characters")
	_, err = server.SetAppGovernanceState(ctx, &pb.SetAppGovernanceStateRequest{AppId: uint32(app.ID), Flags: map[string]string{"long": strings.Repeat("v", 257)}})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "flag values are capped at 256 characters")

	if audit.IsEnterpriseAvailable() {
		require.Eventually(t, func() bool {
			var n int64
			db.Model(&models.AuditRecord{}).Where("resource_type = ? AND resource_id = ?", "app", fmt.Sprint(app.ID)).Count(&n)
			return n == 2
		}, 5*time.Second, 50*time.Millisecond, "each change is audited")
		var rec models.AuditRecord
		require.NoError(t, db.Where("resource_type = ? AND resource_id = ?", "app", fmt.Sprint(app.ID)).Order("id").First(&rec).Error)
		assert.Equal(t, "Plugin Update App Governance State", rec.Action)
		assert.Contains(t, string(rec.Diff), "review lapsed")
		assert.Contains(t, rec.UserName, "asset-catalog")

		// Plugin changes count as mutations for the catalog's history view.
		list, err := server.ListAuditRecords(ctx, &pb.ListAuditRecordsRequest{ResourceType: "app", ResourceId: fmt.Sprint(app.ID), MutationsOnly: true})
		require.NoError(t, err)
		require.Len(t, list.Records, 2)
		assert.Equal(t, "Plugin Update App Governance State", list.Records[0].Action)
	}
}
