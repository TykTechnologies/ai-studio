//go:build enterprise

package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/TykTechnologies/midsommar/v2/enterprise/all"
)

// An Enterprise Studio whose licence fails validation is an error for the
// host, not a process exit, and leaves nothing running.
func TestNewFailsCleanlyWithoutAValidLicence(t *testing.T) {
	opts := newTestOptions(t)
	opts.Config.LicenseKey = "not-a-licence"

	_, err := New(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "licens")

	_, err = New(newTestOptions(t))
	assert.NotErrorIs(t, err, ErrAlreadyRunning, "a failed New must release the instance slot")
}
