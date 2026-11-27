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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	// expiresIn overrides the minted tokens' lifetime in seconds (0: 3600).
	expiresIn atomic.Int32
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
			exp := int32(3600)
			if e := f.expiresIn.Load(); e > 0 {
				exp = e
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": kind + "-" + itoa(n), "token_type": "Bearer", "expires_in": exp,
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
	restore := AllowLoopbackCredentialEndpointsForTests()
	resetTokenSourceCache()
	t.Cleanup(func() {
		restore()
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

func TestTokenSource_IdleEntriesEvicted(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	key := mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))
	_, err := TokenSource(&models.LLM{ID: 20, Name: "old", APIKey: key})
	require.NoError(t, err)
	cacheMu.Lock()
	cache[20].lastUsed = time.Now().Add(-2 * cacheIdleTTL)
	cacheMu.Unlock()

	_, err = TokenSource(&models.LLM{ID: 21, Name: "new", APIKey: key})
	require.NoError(t, err)
	cacheMu.Lock()
	_, stale := cache[20]
	_, fresh := cache[21]
	cacheMu.Unlock()
	assert.False(t, stale, "an LLM unused for longer than the idle TTL is dropped")
	assert.True(t, fresh)
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
	// A file source reads the host's identity: refused while the operator
	// has not opted in, and STS is never called.
	t.Setenv(ADCSetting, "")
	h, err := authHeader(t, &models.LLM{ID: 2, Name: "wif", Vendor: models.VERTEX, APIKey: key})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrHostCredentialSource), err)
	assert.True(t, errors.Is(err, ErrInvalidCredential))
	assert.Contains(t, err.Error(), ADCSetting)
	assert.Empty(t, h)
	assert.Zero(t, g.mints.Load())

	t.Setenv(ADCSetting, "true")
	h, err = authHeader(t, &models.LLM{ID: 2, Name: "wif", Vendor: models.VERTEX, APIKey: key})
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
	t.Setenv(ADCSetting, "")

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
		"userinfo host":      `{"type":"service_account","private_key":"` + secret + `","token_uri":"https://oauth2.googleapis.com@evil.example/token"}`,
		"fragment host":      `{"type":"service_account","private_key":"` + secret + `","token_uri":"https://evil.example#.googleapis.com"}`,
		"lookalike host":     `{"type":"service_account","private_key":"` + secret + `","token_uri":"https://oauth2.googleapis.com.evil.example/token"}`,
		"executable source":  `{"type":"external_account","audience":"x","subject_token_type":"y","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"executable":{"command":"/bin/echo ` + secret + `"}}}`,
		"ssrf url source":    `{"type":"external_account","audience":"x","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"url":"http://10.0.0.5/admin?` + secret + `"}}`,
		"ssrf aws region":    `{"type":"external_account","audience":"x","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"environment_id":"aws1","region_url":"http://internal.example/` + secret + `"}}`,
		"ssrf imdsv2":        `{"type":"external_account","audience":"x","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"environment_id":"aws1","imdsv2_session_token_url":"http://127.0.0.1/` + secret + `"}}`,
		"foreign sts":        `{"type":"external_account","audience":"x","token_url":"https://sts.evil.example/v1/token","credential_source":{"file":"/etc/` + secret + `"}}`,
		"foreign impersonat": `{"type":"impersonated_service_account","service_account_impersonation_url":"https://evil.example/x","source_credentials":{"type":"service_account","private_key":"` + secret + `"}}`,
		"bad source type":    `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/x:generateAccessToken","source_credentials":{"type":"nope","private_key":"` + secret + `"}}`,
	}
	// Point the validator back at the production rule.
	defer SetCredentialURLAllowedForTests(isGoogleEndpoint)()
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

}

// Every Workload Identity Federation credential (Martin, 2026-11-28),
// including one an impersonated_service_account uses as its source, is
// refused unless the operator set AI_STUDIO_VERTEX_ADC=true, both at
// validation and at use.
func TestValidateCredentialJSON_HostSourcedWIFNeedsOptIn(t *testing.T) {
	hostSourced := map[string]string{
		"AWS (environment + metadata)": `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"environment_id":"aws1","region_url":"http://169.254.169.254/latest/meta-data/placement/availability-zone"}}`,
		"AWS (environment only)":       `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"environment_id":"aws1"}}`,
		"Azure (IMDS)":                 `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"url":"http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01","headers":{"Metadata":"True"}}}`,
		"Kubernetes projected token":   `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"file":"/var/run/secrets/tokens/gcp-ksa/token"}}`,
		"x509 certificate":             `{"type":"external_account","token_url":"https://sts.mtls.googleapis.com/v1/token","credential_source":{"certificate":{"use_default_certificate_config":true}}}`,
		"impersonated, WIF source":     `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken","source_credentials":{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"file":"/var/run/token"}}}`,
	}
	for name, key := range hostSourced {
		t.Run(name, func(t *testing.T) {
			t.Setenv(ADCSetting, "")
			_, err := ValidateCredentialJSON(key)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrHostCredentialSource), err)
			assert.Contains(t, err.Error(), ADCSetting)

			t.Setenv(ADCSetting, "true")
			_, err = ValidateCredentialJSON(key)
			assert.NoError(t, err)
		})
	}

	// A service account key reads nothing of the host: no opt-in.
	t.Setenv(ADCSetting, "")
	_, err := ValidateCredentialJSON(mustJSON(t, serviceAccountJSON(t, "https://oauth2.googleapis.com/token")))
	assert.NoError(t, err)
}

// The gate fails closed: it does not depend on recognising the source, and
// an unrecognised or empty source is refused even with the opt-in.
func TestValidateCredentialJSON_WIFGateFailsClosed(t *testing.T) {
	cases := map[string]string{
		"made-up source kind": `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"vault":{"path":"secret/gcp"}}}`,
		"empty source":        `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{}}`,
		"no source":           `{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token"}`,
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(ADCSetting, "")
			_, err := ValidateCredentialJSON(key)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrHostCredentialSource), "needs the setting even with no recognised source: %v", err)

			t.Setenv(ADCSetting, "true")
			_, err = ValidateCredentialJSON(key)
			require.Error(t, err, "an unrecognised or empty source is refused, not accepted")
			assert.True(t, errors.Is(err, ErrInvalidCredential))
			assert.False(t, errors.Is(err, ErrHostCredentialSource))
			assert.Contains(t, err.Error(), "no supported source")
		})
	}

	// Executable sources and non-metadata URLs are refused whatever the setting.
	t.Setenv(ADCSetting, "true")
	_, err := ValidateCredentialJSON(`{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"executable":{"command":"/bin/true"}}}`)
	assert.True(t, errors.Is(err, ErrInvalidCredential))
	assert.Contains(t, err.Error(), "executable")
	_, err = ValidateCredentialJSON(`{"type":"external_account","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"url":"http://10.0.0.5/token"}}`)
	assert.Contains(t, err.Error(), "metadata service")
}

func TestAccessToken_ConcurrentCallsMintOnce(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	llm := &models.LLM{ID: 50, Name: "vx", Vendor: models.VERTEX, APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))}

	var wg sync.WaitGroup
	tokens := make([]string, 32)
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, err := AccessToken(llm)
			assert.NoError(t, err)
			tokens[i] = tok
		}(i)
	}
	wg.Wait()
	for _, tok := range tokens {
		assert.Equal(t, "sa-1", tok)
	}
	assert.EqualValues(t, 1, g.mints.Load(), "concurrent first calls share one mint")
}

func TestAccessToken_RefreshedNearExpiry(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	// oauth2 treats a token as expired 10 s early, so a 5 s token is
	// refreshed on the next call.
	g.expiresIn.Store(5)
	llm := &models.LLM{ID: 51, Name: "vx", Vendor: models.VERTEX, APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))}

	h, err := authHeader(t, llm)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sa-1", h)
	h, err = authHeader(t, llm)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sa-2", h, "an expiring token is replaced")

	g.expiresIn.Store(3600)
	h, _ = authHeader(t, llm)
	assert.Equal(t, "Bearer sa-3", h)
	h, _ = authHeader(t, llm)
	assert.Equal(t, "Bearer sa-3", h, "a fresh token is reused")
}

func TestTokenSource_SharedKeyEntriesAreIndependent(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	key := mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))
	a := &models.LLM{ID: 60, Name: "a", Vendor: models.VERTEX, APIKey: key}
	b := &models.LLM{ID: 61, Name: "b", Vendor: models.VERTEX, APIKey: key}
	tsA, err := TokenSource(a)
	require.NoError(t, err)
	tsB, err := TokenSource(b)
	require.NoError(t, err)

	rotated := *a
	rotated.APIKey = mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))
	tsA2, err := TokenSource(&rotated)
	require.NoError(t, err)
	assert.NotSame(t, tsA, tsA2)

	again, err := TokenSource(b)
	require.NoError(t, err)
	assert.Equal(t, tsB, again, "rotating one LLM's key keeps the other's source")
}
