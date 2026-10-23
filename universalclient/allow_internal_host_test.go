package universalclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// w4e: a tool's calls refuse internal addresses by default (a tool's spec
// can be a community submission); its "Allow internal host" switch exempts
// only its own API host, so a redirect to another internal host is refused.
func TestClient_InternalHostBlockedUnlessAllowed(t *testing.T) {
	netguardtest.BlockInternal(t)
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer other.Close()
	redirect := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect {
			// Another internal host: "localhost" is not the tool's 127.0.0.1.
			http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+r.URL.Path, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"doggie","status":"available"}]`))
	}))
	defer api.Close()
	call := func(c *Client) error {
		_, err := c.CallOperation("findPetsByStatus", map[string][]string{"status": {"available"}}, nil, nil)
		return err
	}

	blocked, err := NewClient(specBytes, api.URL+"/v2")
	require.NoError(t, err)
	assert.ErrorContains(t, call(blocked), "internal address", "blocked by default")

	allowed, err := NewClient(specBytes, api.URL+"/v2", WithAllowInternalHost(true))
	require.NoError(t, err)
	assert.NoError(t, call(allowed), "the tool's own host is exempt")

	redirect = true
	assert.ErrorContains(t, call(allowed), "internal address", "another internal host stays blocked")
}
