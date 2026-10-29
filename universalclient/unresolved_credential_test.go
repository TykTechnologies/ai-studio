package universalclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lsj.7: a tool auth key that did not resolve ($ENV/ not on the allowlist, a
// $SECRET/ that cannot be found, the edge's refused placeholder) fails the
// call; the reference text never reaches the tool's API.
func TestClient_UnresolvedAuthKeyIsNotSent(t *testing.T) {
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer api.Close()

	for _, key := range []string{"$ENV/UPG_TOOL_KEY", "$SECRET/MISSING", secrets.RefusedCredential} {
		c, err := NewClient(specBytes, api.URL+"/v2", WithAllowInternalHost(true), WithAuth("api_key", key))
		require.NoError(t, err)
		_, err = c.CallOperation("findPetsByStatus", map[string][]string{"status": {"available"}}, nil, nil)
		assert.ErrorIs(t, err, secrets.ErrUnresolvedCredential, key)
	}
	assert.Zero(t, calls.Load(), "a request reached the tool's API")

	c, err := NewClient(specBytes, api.URL+"/v2", WithAllowInternalHost(true), WithAuth("api_key", "real-key"))
	require.NoError(t, err)
	_, err = c.CallOperation("findPetsByStatus", map[string][]string{"status": {"available"}}, nil, nil)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
}
