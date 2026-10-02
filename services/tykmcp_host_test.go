//go:build !enterprise

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
)

// The host application's Dashboard connection reaches the service
// InitTykMCP builds. (Community Edition only: the test stands in for the
// Enterprise factory.)
func TestInitTykMCPPassesTheHostConnection(t *testing.T) {
	var got tykmcp.Deps
	tykmcp.RegisterEnterpriseFactory(func(d tykmcp.Deps) tykmcp.Service {
		got = d
		return tykmcp.NewCommunityService()
	})
	t.Cleanup(func() { tykmcp.RegisterEnterpriseFactory(nil) })

	host := &tykmcp.HostConnection{URL: "http://dashboard:3000", Token: func() string { return "key" }}
	s := &Service{}
	s.SetTykMCPHost(host)
	s.InitTykMCP(config.TykMCPConfig{}, "test")
	t.Cleanup(s.TykMCP.Stop)
	assert.Same(t, host, got.Host)
}
