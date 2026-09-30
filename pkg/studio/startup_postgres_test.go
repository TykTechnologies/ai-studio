//go:build !enterprise

package studio

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Studio starts on any DSN its database pool accepts. A DSN without sslmode
// (the Helm chart's and compose file's default) against a server without
// TLS, or with pgx-only options, used to hang New for ever: the cluster
// listener (lib/pq) refused it, and the cleanup then waited on a reader that
// never started.
func TestStartupWithPoolDSNs_Postgres(t *testing.T) {
	base := schemaTestDSN(t)
	for name, params := range map[string]map[string]string{
		"no sslmode":      {"sslmode": ""},
		"pgx-only option": {"sslmode": "", "default_query_exec_mode": "simple_protocol"},
	} {
		t.Run(name, func(t *testing.T) {
			u, err := url.Parse(base)
			require.NoError(t, err)
			q := u.Query()
			for k, v := range params {
				if v == "" {
					q.Del(k)
				} else {
					q.Set(k, v)
				}
			}
			u.RawQuery = q.Encode()

			schema := fmt.Sprintf("studio_startup_%d", time.Now().UnixNano())
			dropSchemaAfter(t, base, schema)
			opts := newTestOptions(t)
			opts.DB = openSchema(t, u.String(), schema)
			// Control mode also starts edge push delivery, the other
			// listener.
			opts.Config.GatewayMode = "control"
			opts.Config.MicrogatewayEncryptionKey = "0123456789abcdef0123456789abcdef"
			opts.Config.GRPCAuthToken = "test-token"
			opts.Config.GRPCTLSEnabled = false

			s := newWithin(t, opts, 2*time.Minute)
			defer stopStudio(t, s)
			assert.True(t, s.clusterLog.Stats().Listening, "the cluster listener connects with the pool's DSN")

			srv := httptest.NewServer(s.HTTPHandler())
			defer srv.Close()
			resp, err := http.Get(srv.URL + "/health")
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// newWithin runs New and fails the test, rather than hanging it, if New
// does not return within d.
func newWithin(t *testing.T, opts Options, d time.Duration) *Studio {
	t.Helper()
	type result struct {
		s   *Studio
		err error
	}
	done := make(chan result, 1)
	go func() {
		s, err := New(opts)
		done <- result{s, err}
	}()
	select {
	case r := <-done:
		require.NoError(t, r.err)
		return r.s
	case <-time.After(d):
		t.Fatalf("New did not return within %s", d)
		return nil
	}
}
