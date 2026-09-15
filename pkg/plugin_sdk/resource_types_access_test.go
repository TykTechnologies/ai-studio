package plugin_sdk

import (
	"context"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessClassPlugin is a minimal ResourceProvider whose registrations and
// instances exercise the tri-state access field.
type accessClassPlugin struct {
	BasePlugin
}

func (p *accessClassPlugin) GetResourceTypeRegistrations() ([]*ResourceTypeRegistration, error) {
	no := false
	return []*ResourceTypeRegistration{
		{Slug: "undeclared", Name: "Undeclared"},
		{Slug: "opted-out", Name: "Opted out", AccessGrantedViaApp: &no, PortalDetailPath: "/portal/plugins/p#/items/{id}"},
	}, nil
}

func (p *accessClassPlugin) ListResourceInstances(ctx Context, slug string) ([]*ResourceInstance, error) {
	yes := true
	return []*ResourceInstance{
		{ID: "a", Name: "A", IsActive: true},
		{ID: "b", Name: "B", IsActive: true, AccessGrantedViaApp: &yes},
	}, nil
}

func (p *accessClassPlugin) GetResourceInstance(ctx Context, slug, id string) (*ResourceInstance, error) {
	no := false
	return &ResourceInstance{ID: id, Name: id, IsActive: true, AccessGrantedViaApp: &no}, nil
}

func (p *accessClassPlugin) ValidateResourceSelection(ctx Context, slug string, ids []string, appID uint32) error {
	return nil
}

func (p *accessClassPlugin) CreateResourceInstance(ctx Context, slug string, payload []byte) (*ResourceInstance, error) {
	return nil, nil
}

// TestWrapper_AccessGrantedViaApp_TriState: nil stays nil on the wire (so the
// platform applies its default), an explicit false stays false, and the
// portal path and instance overrides are copied.
func TestWrapper_AccessGrantedViaApp_TriState(t *testing.T) {
	w := newPluginServerWrapper(&accessClassPlugin{}, RuntimeStudio, nil)

	regs, err := w.GetResourceTypeRegistrations(context.Background(), &pb.GetResourceTypeRegistrationsRequest{})
	require.NoError(t, err)
	require.Len(t, regs.Registrations, 2)

	undeclared, optedOut := regs.Registrations[0], regs.Registrations[1]
	assert.Nil(t, undeclared.AccessGrantedViaApp, "undeclared is not sent as false")
	assert.Empty(t, undeclared.PortalDetailPath)
	require.NotNil(t, optedOut.AccessGrantedViaApp)
	assert.False(t, *optedOut.AccessGrantedViaApp)
	assert.Equal(t, "/portal/plugins/p#/items/{id}", optedOut.PortalDetailPath)

	list, err := w.ListResourceInstances(context.Background(), &pb.ListResourceInstancesRequest{ResourceTypeSlug: "undeclared"})
	require.NoError(t, err)
	require.True(t, list.Success)
	require.Len(t, list.Instances, 2)
	assert.Nil(t, list.Instances[0].AccessGrantedViaApp, "no override inherits the type")
	require.NotNil(t, list.Instances[1].AccessGrantedViaApp)
	assert.True(t, *list.Instances[1].AccessGrantedViaApp)

	one, err := w.GetResourceInstance(context.Background(), &pb.GetResourceInstanceRequest{ResourceTypeSlug: "undeclared", InstanceId: "z"})
	require.NoError(t, err)
	require.True(t, one.Success)
	require.NotNil(t, one.Instance.AccessGrantedViaApp)
	assert.False(t, *one.Instance.AccessGrantedViaApp)
}
