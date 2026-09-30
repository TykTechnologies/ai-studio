//go:build !enterprise

package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Community Edition has no licence: the host sees that, and reloading is a
// no-op. (The Enterprise licence source and reload are tested with the
// Enterprise licensing service.)
func TestLicenseStatus_CommunityEdition(t *testing.T) {
	opts := newTestOptions(t)
	opts.License = func() string { return "" }
	s, err := New(opts)
	require.NoError(t, err)
	defer stopStudio(t, s)

	st := s.LicenseStatus()
	assert.False(t, st.Enterprise)
	assert.True(t, st.Valid)
	assert.Equal(t, -1, st.DaysLeft)
	assert.Empty(t, st.Entitlements)
	assert.NoError(t, s.ReloadLicense())
}
