package edition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckFeaturesNamesEachMissingFeature(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }

	err := checkFeatures([]feature{{"audit", yes}, {"rbac", no}, {"sso", no}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rbac, sso")
	assert.NotContains(t, err.Error(), "audit")
	assert.NoError(t, checkFeatures([]feature{{"audit", yes}}))
}
