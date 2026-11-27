package vertexVendor

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// fakeGoogle serves the token endpoints the credential types call: the
// OAuth2 token endpoint (service_account JWT grant, authorized_user refresh),
// the STS endpoint (external_account) and the IAM credentials endpoint
// (impersonation). Every token it mints is "<kind>-<n>".
type fakeGoogle struct {
	srv   *httptest.Server
	mints atomic.Int32
	fail  atomic.Bool
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	f := &fakeGoogle{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := f.mints.Add(1)
		if f.fail.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`))
			return
		}
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token": // service_account (JWT bearer grant) and authorized_user (refresh)
			kind := "sa"
			if r.Form.Get("grant_type") == "refresh_token" {
				kind = "user"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": kind + "-" + itoa(n), "token_type": "Bearer", "expires_in": 3600,
			})
		case "/sts": // external_account token exchange
			if r.Form.Get("subject_token") != "oidc-subject-token" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sts-" + itoa(n), "token_type": "Bearer", "expires_in": 3600,
				"issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
			})
		case "/impersonate":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sa-") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "imp-" + itoa(n), "expireTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }

// allowLocalEndpoints lets credential JSON point at the fake servers and
// starts each test with an empty cache.
func allowLocalEndpoints(t *testing.T) {
	t.Helper()
	prev := credentialURLAllowed
	credentialURLAllowed = func(u *url.URL) bool { return u.Hostname() == "127.0.0.1" || isGoogleEndpoint(u) }
	resetTokenSourceCache()
	t.Cleanup(func() {
		credentialURLAllowed = prev
		resetTokenSourceCache()
	})
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(k)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func serviceAccountJSON(t *testing.T, tokenURI string) map[string]any {
	return map[string]any{
		"type":           "service_account",
		"project_id":     "proj",
		"private_key_id": "kid-1",
		"private_key":    testPrivateKeyPEM(t),
		"client_email":   "sa@proj.iam.gserviceaccount.com",
		"client_id":      "123",
		"token_uri":      tokenURI,
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func authHeader(t *testing.T, llm *models.LLM) (string, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/projects/p/locations/l/publishers/google/models/m:generateContent", nil)
	err := (&Vertex{}).ProxySetAuthHeader(r, llm)
	return r.Header.Get("Authorization"), err
}

func TestProxySetAuthHeader_ServiceAccountMintsAndReusesToken(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	llm := &models.LLM{ID: 1, Name: "vx", Vendor: models.VERTEX, APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))}

	for i := 0; i < 5; i++ {
		h, err := authHeader(t, llm)
		require.NoError(t, err)
		assert.Equal(t, "Bearer sa-1", h)
	}
	assert.EqualValues(t, 1, g.mints.Load(), "one mint for N calls while the token is valid")
}

func TestProxySetAuthHeader_RotatedKeyGetsNewSource(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	llm := &models.LLM{ID: 7, Name: "vx", Vendor: models.VERTEX, APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))}
	h, err := authHeader(t, llm)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sa-1", h)

	rotated := *llm
	rotated.APIKey = mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))
	h, err = authHeader(t, &rotated)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sa-2", h, "a new key mints a new token")

	cacheMu.Lock()
	n := len(cache)
	cacheMu.Unlock()
	assert.Equal(t, 1, n, "the old key's source is evicted")

	// Another LLM with the first key has its own source.
	other := *llm
	other.ID = 8
	h, err = authHeader(t, &other)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sa-3", h)
}

func TestProxySetAuthHeader_ExternalAccountUsesSTS(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	subject := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(subject, []byte("oidc-subject-token"), 0o600))
	key := mustJSON(t, map[string]any{
		"type":               "external_account",
		"audience":           "//iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/pool/providers/prov",
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          g.srv.URL + "/sts",
		"credential_source":  map[string]any{"file": subject},
	})
	h, err := authHeader(t, &models.LLM{ID: 2, Name: "wif", Vendor: models.VERTEX, APIKey: key})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sts-1", h)
}

func TestProxySetAuthHeader_ImpersonatedServiceAccount(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	key := mustJSON(t, map[string]any{
		"type":                              "impersonated_service_account",
		"service_account_impersonation_url": g.srv.URL + "/impersonate",
		"source_credentials":                serviceAccountJSON(t, g.srv.URL+"/token"),
	})
	h, err := authHeader(t, &models.LLM{ID: 3, Name: "imp", Vendor: models.VERTEX, APIKey: key})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "Bearer imp-"), h)
}

func TestProxySetAuthHeader_AuthorizedUser(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	key := mustJSON(t, map[string]any{
		"type": "authorized_user", "client_id": "cid", "client_secret": "csecret",
		"refresh_token": "rtok", "token_uri": g.srv.URL + "/token",
	})
	h, err := authHeader(t, &models.LLM{ID: 4, Name: "user", Vendor: models.VERTEX, APIKey: key})
	require.NoError(t, err)
	assert.Equal(t, "Bearer user-1", h)
}

func TestProxySetAuthHeader_AccessTokenPassedThrough(t *testing.T) {
	allowLocalEndpoints(t)
	h, err := authHeader(t, &models.LLM{ID: 5, Name: "tok", Vendor: models.VERTEX, APIKey: " ya29.a0AfB_test \n"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer ya29.a0AfB_test", h)
}

func TestProxySetAuthHeader_EmptyKeyRefusedWithoutHostLookup(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	// Host credentials that ADC would find must not be read on this path.
	adcFile := filepath.Join(t.TempDir(), "adc.json")
	require.NoError(t, os.WriteFile(adcFile, []byte(mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))), 0o600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcFile)

	h, err := authHeader(t, &models.LLM{ID: 6, Name: "empty", Vendor: models.VERTEX})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoCredential), err)
	assert.Contains(t, err.Error(), ADCSetting)
	assert.Contains(t, err.Error(), "credential JSON")
	assert.Empty(t, h, "no Authorization header is set")
	assert.Zero(t, g.mints.Load(), "no token endpoint is contacted")
}

func TestProxySetAuthHeader_InvalidKeysErrorWithoutKeyMaterial(t *testing.T) {
	allowLocalEndpoints(t)
	const secret = "s3cr3t-material"
	cases := map[string]string{
		"not json":           "sk-" + secret,
		"broken json":        `{"type":"service_account","private_key":"` + secret,
		"unknown type":       `{"type":"gdch_service_account","private_key":"` + secret + `"}`,
		"no type":            `{"private_key":"` + secret + `"}`,
		"bad private key":    `{"type":"service_account","client_email":"a@b","private_key":"` + secret + `","token_uri":"https://oauth2.googleapis.com/token"}`,
		"no client_email":    `{"type":"service_account","private_key":"` + secret + `"}`,
		"foreign token_uri":  `{"type":"service_account","private_key":"` + secret + `","token_uri":"https://evil.example/token"}`,
		"http google host":   `{"type":"service_account","private_key":"` + secret + `","token_uri":"http://oauth2.googleapis.com/token"}`,
		"lookalike host":     `{"type":"service_account","private_key":"` + secret + `","token_uri":"https://oauth2.googleapis.com.evil.example/token"}`,
		"executable source":  `{"type":"external_account","audience":"x","subject_token_type":"y","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"executable":{"command":"/bin/echo ` + secret + `"}}}`,
		"foreign sts":        `{"type":"external_account","audience":"x","token_url":"https://sts.evil.example/v1/token","credential_source":{"file":"/etc/` + secret + `"}}`,
		"foreign impersonat": `{"type":"impersonated_service_account","service_account_impersonation_url":"https://evil.example/x","source_credentials":{"type":"service_account","private_key":"` + secret + `"}}`,
		"bad source type":    `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/x:generateAccessToken","source_credentials":{"type":"nope","private_key":"` + secret + `"}}`,
	}
	// Point the validator back at the production rule.
	credentialURLAllowed = isGoogleEndpoint
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := authHeader(t, &models.LLM{ID: 9, Name: "bad", Vendor: models.VERTEX, APIKey: key})
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidCredential), err)
			assert.NotContains(t, err.Error(), secret)
		})
	}
}

func TestProxySetAuthHeader_MintFailureIsClearAndClean(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	g.fail.Store(true)
	sa := serviceAccountJSON(t, g.srv.URL+"/token")
	h, err := authHeader(t, &models.LLM{ID: 10, Name: "vx", Vendor: models.VERTEX, APIKey: mustJSON(t, sa)})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTokenMint), err)
	assert.NotContains(t, err.Error(), "PRIVATE KEY")
	assert.NotContains(t, err.Error(), "Invalid JWT Signature", "upstream detail is logged, not returned")
	assert.Empty(t, h)
}

func TestSetTokenSourceOverride(t *testing.T) {
	allowLocalEndpoints(t)
	restore := SetTokenSourceOverride(func(llm *models.LLM) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake-for-" + llm.Name}), nil
	})
	h, err := authHeader(t, &models.LLM{ID: 11, Name: "x", Vendor: models.VERTEX})
	require.NoError(t, err)
	assert.Equal(t, "Bearer fake-for-x", h)

	restore()
	_, err = authHeader(t, &models.LLM{ID: 11, Name: "x", Vendor: models.VERTEX})
	assert.True(t, errors.Is(err, ErrNoCredential))
}

func TestValidateCredentialJSON_ProductionHosts(t *testing.T) {
	credType, err := ValidateCredentialJSON(mustJSON(t, serviceAccountJSON(t, "https://oauth2.googleapis.com/token")))
	require.NoError(t, err)
	assert.EqualValues(t, "service_account", credType)

	_, err = ValidateCredentialJSON(`{"type":"authorized_user","token_uri":"https://accounts.google.com/o/oauth2/token"}`)
	require.NoError(t, err)

	_, err = ValidateCredentialJSON(`{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"environment_id":"aws1","region_url":"http://169.254.169.254/latest/meta-data/placement/availability-zone"}}`)
	require.NoError(t, err, "AWS WIF reads the instance metadata server; only Google endpoints are pinned")
}
