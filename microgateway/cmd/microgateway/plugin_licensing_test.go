package main

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	pb "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validLicense struct{}

func (validLicense) IsValid() bool { return true }
func (validLicense) DaysLeft() int { return 42 }

// wirePluginLicensing is what makes the plugin manager willing to start
// plugins (plugins.ErrLicensingNotConfigured), and what lets them see the
// license instead of "community". main calls it right after it builds the
// service container.
func TestWirePluginLicensing(t *testing.T) {
	server := services.NewMicrogatewayManagementServer(nil, nil, nil, nil, nil)
	pm := plugins.NewPluginManager(nil)
	pm.SetManagementServer(server)
	require.False(t, server.LicensingConfigured())

	before, err := server.GetLicenseInfo(context.Background(), &pb.GetLicenseInfoRequest{Context: &pb.PluginContext{PluginId: 1}})
	require.NoError(t, err)
	assert.Equal(t, "community", before.LicenseType, "what a plugin started too early was told")

	wirePluginLicensing(pm, validLicense{})

	assert.True(t, server.LicensingConfigured())
	after, err := server.GetLicenseInfo(context.Background(), &pb.GetLicenseInfoRequest{Context: &pb.PluginContext{PluginId: 1}})
	require.NoError(t, err)
	assert.Equal(t, "enterprise", after.LicenseType)
	assert.True(t, after.LicenseValid)
}
