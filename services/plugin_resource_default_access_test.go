package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeResourcePluginClient answers ListResourceInstances from a fixed map;
// every other plugin RPC is left unimplemented.
type fakeResourcePluginClient struct {
	pb.PluginServiceClient
	instances map[string][]*pb.ResourceInstanceProto // slug → instances
}

func (f *fakeResourcePluginClient) ListResourceInstances(ctx context.Context, req *pb.ListResourceInstancesRequest, opts ...grpc.CallOption) (*pb.ListResourceInstancesResponse, error) {
	return &pb.ListResourceInstancesResponse{Success: true, Instances: f.instances[req.ResourceTypeSlug]}, nil
}

// withLoadedResourcePlugin attaches a plugin manager whose plugin serves the
// given instances, so registration and instance-changed reconciliation run
// against it.
func withLoadedResourcePlugin(service *Service, pluginID uint, instances map[string][]*pb.ResourceInstanceProto) *fakeResourcePluginClient {
	if service.AIStudioPluginManager == nil {
		service.AIStudioPluginManager = &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{}}
	}
	client := &fakeResourcePluginClient{instances: instances}
	service.AIStudioPluginManager.loadedPlugins[pluginID] = &LoadedAIStudioPlugin{GRPCClient: client, IsHealthy: true}
	return client
}

func defaultGroupInstanceIDs(t *testing.T, service *Service, typeID uint) []string {
	t.Helper()
	group, err := models.GetOrCreateDefaultGroup(service.DB)
	require.NoError(t, err)
	var ids []string
	require.NoError(t, service.DB.Model(&models.GroupPluginResource{}).
		Where("group_id = ? AND plugin_resource_type_id = ?", group.ID, typeID).
		Order("instance_id").Pluck("instance_id", &ids).Error)
	return ids
}

func TestNormalizeDefaultAccess(t *testing.T) {
	assert.Equal(t, models.DefaultAccessAuto, models.NormalizeDefaultAccess(""))
	assert.Equal(t, models.DefaultAccessAuto, models.NormalizeDefaultAccess("auto"))
	assert.Equal(t, models.DefaultAccessAuto, models.NormalizeDefaultAccess("bogus"))
	assert.Equal(t, models.DefaultAccessExplicit, models.NormalizeDefaultAccess(" Explicit "))
}

// Community Edition has no team segmentation: the Default grant is the only
// route to users, so it is on whatever a plugin declares. Enterprise honours
// an explicit declaration and keeps the grant for everyone else.
func TestAutoGrantsDefaultGroup_FollowsEditionAndDeclaration(t *testing.T) {
	auto := &models.PluginResourceType{DefaultAccess: models.DefaultAccessAuto}
	undeclared := &models.PluginResourceType{}
	explicit := &models.PluginResourceType{DefaultAccess: models.DefaultAccessExplicit}

	assert.True(t, autoGrantsDefaultGroup(auto))
	assert.True(t, autoGrantsDefaultGroup(undeclared))
	assert.True(t, autoGrantsDefaultGroup(nil))
	if group_access.IsFilteringEnabled() {
		assert.False(t, autoGrantsDefaultGroup(explicit), "Enterprise: explicit types are never auto-granted")
	} else {
		assert.True(t, autoGrantsDefaultGroup(explicit), "CE: explicit is forced to auto so instances cannot vanish")
	}
}

func TestRegisterPluginResourceTypes_DefaultAccess(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	_, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)

	autoPlugin := createTestPlugin(t, db, "auto-plugin")
	explicitPlugin := createTestPlugin(t, db, "explicit-plugin")
	instances := map[string][]*pb.ResourceInstanceProto{
		"items": {
			{Id: "a", Name: "A", IsActive: true},
			{Id: "b", Name: "B", IsActive: true},
			{Id: "c", Name: "C", IsActive: false},
		},
	}
	withLoadedResourcePlugin(service, autoPlugin.ID, instances)
	withLoadedResourcePlugin(service, explicitPlugin.ID, instances)

	require.NoError(t, service.RegisterPluginResourceTypes(autoPlugin.ID, []models.PluginResourceType{{Slug: "items", Name: "Items"}}))
	require.NoError(t, service.RegisterPluginResourceTypes(explicitPlugin.ID, []models.PluginResourceType{{Slug: "items", Name: "Items", DefaultAccess: "explicit"}}))

	autoType, err := service.GetPluginResourceTypeByPluginAndSlug(autoPlugin.ID, "items")
	require.NoError(t, err)
	explicitType, err := service.GetPluginResourceTypeByPluginAndSlug(explicitPlugin.ID, "items")
	require.NoError(t, err)
	assert.Equal(t, models.DefaultAccessAuto, autoType.DefaultAccess, "undeclared is stored as auto")
	assert.Equal(t, models.DefaultAccessExplicit, explicitType.DefaultAccess)

	assert.Equal(t, []string{"a", "b"}, defaultGroupInstanceIDs(t, service, autoType.ID), "active instances of an auto type join Default at registration")
	if group_access.IsFilteringEnabled() {
		assert.Empty(t, defaultGroupInstanceIDs(t, service, explicitType.ID), "Enterprise: explicit types are not granted")
	} else {
		assert.Equal(t, []string{"a", "b"}, defaultGroupInstanceIDs(t, service, explicitType.ID), "CE: explicit behaves as auto")
	}

	t.Run("re-registration updates the declaration", func(t *testing.T) {
		require.NoError(t, service.RegisterPluginResourceTypes(explicitPlugin.ID, []models.PluginResourceType{{Slug: "items", Name: "Items"}}))
		updated, err := service.GetPluginResourceTypeByPluginAndSlug(explicitPlugin.ID, "items")
		require.NoError(t, err)
		assert.Equal(t, models.DefaultAccessAuto, updated.DefaultAccess)
	})
}

// An instance created after the plugin loaded used to stay invisible to
// non-admins until the next plugin load; the instance-changed event now
// grants it to Default straight away (auto types only).
func TestRefreshInstanceDetails_GrantsNewInstancesOfAutoTypes(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	_, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)

	autoPlugin := createTestPlugin(t, db, "auto-plugin")
	explicitPlugin := createTestPlugin(t, db, "explicit-plugin")
	autoClient := withLoadedResourcePlugin(service, autoPlugin.ID, map[string][]*pb.ResourceInstanceProto{})
	explicitClient := withLoadedResourcePlugin(service, explicitPlugin.ID, map[string][]*pb.ResourceInstanceProto{})
	require.NoError(t, service.RegisterPluginResourceTypes(autoPlugin.ID, []models.PluginResourceType{{Slug: "items", Name: "Items"}}))
	require.NoError(t, service.RegisterPluginResourceTypes(explicitPlugin.ID, []models.PluginResourceType{{Slug: "items", Name: "Items", DefaultAccess: models.DefaultAccessExplicit}}))
	autoType, _ := service.GetPluginResourceTypeByPluginAndSlug(autoPlugin.ID, "items")
	explicitType, _ := service.GetPluginResourceTypeByPluginAndSlug(explicitPlugin.ID, "items")

	// Both plugins use the slug "items"; each instance lives in one plugin.
	autoClient.instances["items"] = []*pb.ResourceInstanceProto{{Id: "new-auto", IsActive: true}, {Id: "draft", IsActive: false}}
	explicitClient.instances["items"] = []*pb.ResourceInstanceProto{{Id: "new-explicit", IsActive: true}}

	service.refreshInstanceDetails("items", "new-auto")
	service.refreshInstanceDetails("items", "draft")
	service.refreshInstanceDetails("items", "new-explicit")

	assert.Equal(t, []string{"new-auto"}, defaultGroupInstanceIDs(t, service, autoType.ID), "the owning plugin's type is granted; inactive instances are not")
	if group_access.IsFilteringEnabled() {
		assert.Empty(t, defaultGroupInstanceIDs(t, service, explicitType.ID))
	} else {
		assert.Equal(t, []string{"new-explicit"}, defaultGroupInstanceIDs(t, service, explicitType.ID))
	}
}

func TestSetResourceInstanceGroups(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "catalog")
	other := createTestPlugin(t, db, "other")
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{Slug: "agent", Name: "Agent", DefaultAccess: models.DefaultAccessExplicit}}))
	require.NoError(t, service.RegisterPluginResourceTypes(other.ID, []models.PluginResourceType{{Slug: "secret", Name: "Secret"}}))

	g1 := &models.Group{Name: "Platform"}
	g2 := &models.Group{Name: "Marketing"}
	require.NoError(t, db.Create(g1).Error)
	require.NoError(t, db.Create(g2).Error)

	got, err := service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", []uint{g1.ID}, false)
	require.NoError(t, err)
	assert.Equal(t, []uint{g1.ID}, got)

	got, err = service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", []uint{g2.ID}, false)
	require.NoError(t, err)
	assert.Equal(t, []uint{g1.ID, g2.ID}, got, "add keeps existing grants")

	got, err = service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", []uint{g2.ID}, true)
	require.NoError(t, err)
	assert.Equal(t, []uint{g2.ID}, got, "replace drops the others")

	got, err = service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", nil, true)
	require.NoError(t, err)
	assert.Empty(t, got, "replace with nothing revokes every grant")

	got, err = service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", []uint{g2.ID}, false)
	require.NoError(t, err, "a revoked grant can be given again (no tombstone in the unique index)")
	assert.Equal(t, []uint{g2.ID}, got)

	read, err := service.ResourceInstanceGroups(plugin.ID, "agent", "ast_1")
	require.NoError(t, err)
	assert.Equal(t, []uint{g2.ID}, read)

	_, err = service.SetResourceInstanceGroups(plugin.ID, "secret", "x", []uint{g1.ID}, false)
	assert.ErrorIs(t, err, ErrResourceTypeNotOwned, "another plugin's type")
	_, err = service.ResourceInstanceGroups(plugin.ID, "secret", "x")
	assert.ErrorIs(t, err, ErrResourceTypeNotOwned)
	_, err = service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", []uint{99999}, false)
	assert.ErrorIs(t, err, ErrUnknownGroup)

	groups, err := service.ListGroupsForPlugin()
	require.NoError(t, err)
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	assert.Contains(t, names, "Platform")
	assert.Contains(t, names, "Marketing")
}

func TestAccessibleResourceInstanceIDs(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "catalog")
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{Slug: "agent", Name: "Agent", DefaultAccess: models.DefaultAccessExplicit}}))

	team := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(team).Error)
	member := &models.User{Email: "member@example.com", Name: "Member"}
	outsider := &models.User{Email: "outsider@example.com", Name: "Outsider"}
	admin := &models.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true}
	for _, u := range []*models.User{member, outsider, admin} {
		require.NoError(t, db.Create(u).Error)
	}
	require.NoError(t, service.AddUserToGroup(member.ID, team.ID))
	_, err := service.SetResourceInstanceGroups(plugin.ID, "agent", "ast_1", []uint{team.ID}, false)
	require.NoError(t, err)

	ctx := context.Background()
	seeAll, ids, err := service.AccessibleResourceInstanceIDs(ctx, plugin.ID, "agent", member.ID)
	require.NoError(t, err)
	assert.False(t, seeAll)
	assert.Equal(t, []string{"ast_1"}, ids)

	seeAll, ids, err = service.AccessibleResourceInstanceIDs(ctx, plugin.ID, "agent", outsider.ID)
	require.NoError(t, err)
	assert.False(t, seeAll)
	assert.Empty(t, ids)

	seeAll, _, err = service.AccessibleResourceInstanceIDs(ctx, plugin.ID, "agent", admin.ID)
	require.NoError(t, err)
	assert.True(t, seeAll, "a full administrator manages teams and sees every instance")

	_, ids, err = service.AccessibleResourceInstanceIDs(ctx, plugin.ID, "agent", 424242)
	require.NoError(t, err)
	assert.Empty(t, ids, "an unknown user sees nothing")

	_, _, err = service.AccessibleResourceInstanceIDs(ctx, plugin.ID, "unknown", member.ID)
	assert.ErrorIs(t, err, ErrResourceTypeNotOwned)
}
