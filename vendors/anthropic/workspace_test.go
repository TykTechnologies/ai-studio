package anthropicVendor

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The workspace an upstream call runs in is the LLM's pin, never the
// client's: client workspace and user-profile headers are dropped.
func TestProxySetAuthHeaderPinsWorkspace(t *testing.T) {
	for name, tc := range map[string]struct {
		pin  string
		want []string
	}{
		"pinned":     {pin: "wrkspc_pinned", want: []string{"wrkspc_pinned"}},
		"not pinned": {pin: "", want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
			require.NoError(t, err)
			r.Header.Add("anthropic-workspace-id", "wrkspc_client")
			r.Header.Add("Anthropic-Workspace-Id", "wrkspc_client_2")
			r.Header.Set("Anthropic-User-Profile-Id", "prof_client")
			require.NoError(t, (&Anthropic{}).ProxySetAuthHeader(r, &models.LLM{APIKey: "k", VendorWorkspaceID: tc.pin}))
			assert.Equal(t, "k", r.Header.Get("x-api-key"))
			assert.Equal(t, tc.want, r.Header.Values("anthropic-workspace-id"))
			assert.Empty(t, r.Header.Values("anthropic-user-profile-id"))
		})
	}
}
