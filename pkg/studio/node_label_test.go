//go:build !enterprise

package studio

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A full Studio registers under its host's label, or "studio", and as a
// replica that may lead; a label the registry cannot hold stops New.
func TestStudioRegistersItsNodeLabel(t *testing.T) {
	opts := newTestOptions(t)
	s, err := New(opts)
	require.NoError(t, err)
	var node models.ClusterNode
	require.NoError(t, opts.DB.First(&node, "node_id = ?", s.clusterNode.ID()).Error)
	assert.Equal(t, DefaultNodeLabel, node.Label)
	assert.True(t, node.CanLead())
	require.NotNil(t, node.LeaderEligible, "eligibility is recorded, not left to the older-row default")
	stopStudio(t, s)

	opts = newTestOptions(t)
	opts.NodeLabel = "dashboard"
	s, err = New(opts)
	require.NoError(t, err)
	node = models.ClusterNode{}
	require.NoError(t, opts.DB.First(&node, "node_id = ?", s.clusterNode.ID()).Error)
	assert.Equal(t, "dashboard", node.Label)
	stopStudio(t, s)

	opts = newTestOptions(t)
	opts.NodeLabel = strings.Repeat("x", 65)
	_, err = New(opts)
	assert.ErrorContains(t, err, "label")
}
