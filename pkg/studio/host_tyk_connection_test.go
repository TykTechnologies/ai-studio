//go:build !enterprise

package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Community Edition has no Tyk Dashboard integration: a valid
// HostTykConnection is ignored (and logged), not an error.
func TestNewIgnoresHostTykConnectionInCommunityEdition(t *testing.T) {
	opts := newTestOptions(t)
	opts.HostTykConnection = &HostTykConnection{URL: "http://localhost:3000", Token: func() string { return "key" }}
	s, err := New(opts)
	require.NoError(t, err)
	stopStudio(t, s)

	var count int64
	require.NoError(t, opts.DB.Model(&models.TykConnection{}).Count(&count).Error)
	assert.Zero(t, count)
}
