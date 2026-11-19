package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Every pending-change type with an owning Workspace maps to a catalogue
// resource of a Workspace scope kind, so the hidden set follows the
// catalogue's ReadScoped kinds and cannot drift from the route gate.
func TestPendingTypeResourceCoversEveryStampedType(t *testing.T) {
	for _, typ := range services.PendingStampedTypes() {
		res, ok := pendingTypeResource[typ]
		require.True(t, ok, "pending type %q has no resource", typ)
		assert.True(t, authz.Read(res).Valid(), "%q is not a catalogue resource", res)
		assert.True(t, authz.ResourceScope(res).InWorkspace(), "%q is not a Workspace resource", res)
	}
	// Today's hidden set: the ReadScoped kinds among the sources.
	var hidden []string
	for _, typ := range services.PendingStampedTypes() {
		if authz.ResourceScope(pendingTypeResource[typ]).ReadScoped() {
			hidden = append(hidden, typ)
		}
	}
	assert.ElementsMatch(t, []string{"app", "model_router", "semantic_router"}, hidden)
}
