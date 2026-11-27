package vertexVendor

// Vertex upstream credentials: the explicit part of the credential chain
// (epic midsommar-clean-1af, design section "Credential chain").
//
//  1. A Google credential JSON in the LLM's API key: service_account,
//     external_account (Workload Identity Federation), impersonated_service_account
//     or authorized_user. Tokens are minted with golang.org/x/oauth2/google.
//  2. A raw access token ("ya29." prefix), passed through as-is: tests and
//     short manual use only, it expires within the hour.
//  3. Host Application Default Credentials, only when an operator opts in
//     (a separate task; until then an empty key is refused).
//  4. Otherwise an error naming both options. A request is never sent
//     unauthenticated.
//
// The JSON comes from whoever may edit the LLM, so it is validated before it
// reaches the Google library (Google's guidance for externally sourced
// credential configurations): only the four types above, token and
// impersonation endpoints on Google hosts over HTTPS, and no executable
// credential sources.

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// CloudPlatformScope is the OAuth2 scope Vertex AI requests need.
const CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// AccessTokenPrefix marks a raw Google OAuth2 access token.
const AccessTokenPrefix = "ya29."

// ADCSetting names the operator setting that lets a key-less Vertex LLM use
// the host's Application Default Credentials.
const ADCSetting = "AI_STUDIO_VERTEX_ADC"

// tokenFetchTimeout bounds one call to a token endpoint, so a slow STS
// cannot hold a proxied request indefinitely.
const tokenFetchTimeout = 30 * time.Second

// Errors callers can tell apart with errors.Is. Their text never contains
// credential material.
var (
	// ErrNoCredential: the key is empty and host ADC is not enabled.
	ErrNoCredential = errors.New("vertex: no credential configured")
	// ErrInvalidCredential: the key is neither a supported Google
	// credential JSON nor an access token.
	ErrInvalidCredential = errors.New("vertex: invalid credential")
	// ErrTokenMint: the credential parsed, but no access token could be
	// obtained from Google (bad key, revoked account, STS refused, timeout).
	ErrTokenMint = errors.New("vertex: could not obtain a Google access token")
)

// supportedCredentialTypes are the Google credential JSON types accepted in
// a Vertex LLM's key.
var supportedCredentialTypes = map[string]google.CredentialsType{
	string(google.ServiceAccount):             google.ServiceAccount,
	string(google.ExternalAccount):            google.ExternalAccount,
	string(google.ImpersonatedServiceAccount): google.ImpersonatedServiceAccount,
	string(google.AuthorizedUser):             google.AuthorizedUser,
}

// credentialURLAllowed reports whether a token, STS or impersonation
// endpoint named in a credential JSON may be called. Tests replace it to
// point credentials at local fake endpoints.
var credentialURLAllowed = isGoogleEndpoint

// tokenHTTPClient fetches tokens. Tests may replace it.
var tokenHTTPClient = &http.Client{Timeout: tokenFetchTimeout}

// isGoogleEndpoint accepts https URLs on googleapis.com subdomains and
// accounts.google.com (older authorized_user files name it as token_uri).
func isGoogleEndpoint(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "accounts.google.com" || strings.HasSuffix(host, ".googleapis.com")
}

// credentialFile holds the fields of a Google credential JSON that are
// validated before the JSON reaches the Google library.
type credentialFile struct {
	Type                           string          `json:"type"`
	ClientEmail                    string          `json:"client_email"`
	PrivateKey                     string          `json:"private_key"`
	TokenURI                       string          `json:"token_uri"`
	TokenURL                       string          `json:"token_url"`
	TokenInfoURL                   string          `json:"token_info_url"`
	ServiceAccountImpersonationURL string          `json:"service_account_impersonation_url"`
	CredentialSource               json.RawMessage `json:"credential_source"`
	SourceCredentials              json.RawMessage `json:"source_credentials"`
}

// IsAccessToken reports whether key is a raw Google access token.
func IsAccessToken(key string) bool {
	return strings.HasPrefix(strings.TrimSpace(key), AccessTokenPrefix)
}

// LooksLikeCredentialJSON reports whether key is shaped like a JSON object,
// which is how a credential JSON is told from an access token or garbage.
func LooksLikeCredentialJSON(key string) bool {
	return strings.HasPrefix(strings.TrimSpace(key), "{")
}

// ValidateCredentialJSON checks a Google credential JSON without contacting
// Google and returns its type. The error never quotes the key.
func ValidateCredentialJSON(key string) (google.CredentialsType, error) {
	return validateCredentialJSON([]byte(strings.TrimSpace(key)), 0)
}

func validateCredentialJSON(raw []byte, depth int) (google.CredentialsType, error) {
	var f credentialFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return "", fmt.Errorf("%w: the API key is not valid JSON; expected a Google credential JSON (service account key or Workload Identity Federation config) or an access token", ErrInvalidCredential)
	}
	if f.Type == "" {
		return "", fmt.Errorf("%w: the credential JSON has no \"type\" field", ErrInvalidCredential)
	}
	credType, ok := supportedCredentialTypes[f.Type]
	if !ok {
		return "", fmt.Errorf("%w: unsupported credential type %q (supported: service_account, external_account, impersonated_service_account, authorized_user)", ErrInvalidCredential, f.Type)
	}
	for field, raw := range map[string]string{
		"token_uri":                         f.TokenURI,
		"token_url":                         f.TokenURL,
		"token_info_url":                    f.TokenInfoURL,
		"service_account_impersonation_url": f.ServiceAccountImpersonationURL,
	} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || !credentialURLAllowed(u) {
			return "", fmt.Errorf("%w: %s must be an https Google endpoint (*.googleapis.com)", ErrInvalidCredential, field)
		}
	}
	switch credType {
	case google.ServiceAccount:
		if f.ClientEmail == "" {
			return "", fmt.Errorf("%w: service_account credential has no client_email", ErrInvalidCredential)
		}
		if !validPrivateKey(f.PrivateKey) {
			return "", fmt.Errorf("%w: service_account credential has no valid private_key (expected a PEM-encoded RSA key)", ErrInvalidCredential)
		}
	case google.ExternalAccount:
		if f.TokenURL == "" {
			return "", fmt.Errorf("%w: external_account credential has no token_url", ErrInvalidCredential)
		}
		var src struct {
			Executable json.RawMessage `json:"executable"`
		}
		if len(f.CredentialSource) > 0 {
			if err := json.Unmarshal(f.CredentialSource, &src); err != nil {
				return "", fmt.Errorf("%w: external_account credential_source is not a JSON object", ErrInvalidCredential)
			}
		}
		if len(src.Executable) > 0 && string(src.Executable) != "null" {
			return "", fmt.Errorf("%w: executable-sourced external_account credentials are not supported", ErrInvalidCredential)
		}
	case google.ImpersonatedServiceAccount:
		if f.ServiceAccountImpersonationURL == "" || len(f.SourceCredentials) == 0 {
			return "", fmt.Errorf("%w: impersonated_service_account credential needs service_account_impersonation_url and source_credentials", ErrInvalidCredential)
		}
		if depth > 0 {
			return "", fmt.Errorf("%w: nested impersonated_service_account credentials are not supported", ErrInvalidCredential)
		}
		if _, err := validateCredentialJSON(f.SourceCredentials, depth+1); err != nil {
			return "", fmt.Errorf("source_credentials: %w", err)
		}
	}
	return credType, nil
}

// validPrivateKey reports whether s is a PEM-encoded PKCS#8 or PKCS#1 RSA
// key, as in a service account key file.
func validPrivateKey(s string) bool {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return false
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		_, ok := k.(*rsa.PrivateKey)
		return ok
	}
	_, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	return err == nil
}

// TokenSourceFunc builds the token source for a Vertex LLM. See
// SetTokenSourceOverride.
type TokenSourceFunc func(llm *models.LLM) (oauth2.TokenSource, error)

var (
	overrideMu sync.RWMutex
	override   TokenSourceFunc
)

// SetTokenSourceOverride replaces credential resolution for every Vertex
// LLM, so tests outside this package (proxy, switches, microgateway) can
// serve a fake token without a real Google credential. It returns a
// function restoring the previous behaviour. For tests only.
func SetTokenSourceOverride(fn TokenSourceFunc) (restore func()) {
	overrideMu.Lock()
	prev := override
	override = fn
	overrideMu.Unlock()
	return func() {
		overrideMu.Lock()
		override = prev
		overrideMu.Unlock()
	}
}

// cachedSource is one LLM's token source, valid while the key hashes to
// keyHash.
type cachedSource struct {
	keyHash [sha256.Size]byte
	ts      oauth2.TokenSource
}

var (
	cacheMu sync.Mutex
	cache   = map[uint]*cachedSource{}
)

// TokenSource returns the token source for a Vertex LLM's credential: a
// cached, self-refreshing source for a credential JSON, a static source for
// an access token. One source is kept per LLM ID; a changed key replaces it.
// Building a source does not contact Google: the first Token call does.
func TokenSource(llm *models.LLM) (oauth2.TokenSource, error) {
	if llm == nil {
		return nil, fmt.Errorf("%w: no LLM", ErrNoCredential)
	}
	overrideMu.RLock()
	fn := override
	overrideMu.RUnlock()
	if fn != nil {
		return fn(llm)
	}

	key := strings.TrimSpace(llm.APIKey)
	switch {
	case key == "":
		return nil, fmt.Errorf("%w for LLM %q: set its API key to a Google credential JSON (a service account key or a Workload Identity Federation config), or have the operator enable host Application Default Credentials with %s=true", ErrNoCredential, llm.Name, ADCSetting)
	case IsAccessToken(key):
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: key, TokenType: "Bearer"}), nil
	case !LooksLikeCredentialJSON(key):
		return nil, fmt.Errorf("%w for LLM %q: the API key must be a Google credential JSON (a service account key or a Workload Identity Federation config) or an access token starting with %q", ErrInvalidCredential, llm.Name, AccessTokenPrefix)
	}

	hash := sha256.Sum256([]byte(key))
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if c, ok := cache[llm.ID]; ok && c.keyHash == hash {
		return c.ts, nil
	}
	ts, err := newCredentialTokenSource(key)
	if err != nil {
		return nil, fmt.Errorf("LLM %q: %w", llm.Name, err)
	}
	// A rotated key replaces the LLM's previous source.
	cache[llm.ID] = &cachedSource{keyHash: hash, ts: ts}
	return ts, nil
}

// newCredentialTokenSource validates a credential JSON and builds its
// refreshing token source.
func newCredentialTokenSource(key string) (oauth2.TokenSource, error) {
	credType, err := ValidateCredentialJSON(key)
	if err != nil {
		return nil, err
	}
	// The source refreshes long after this call returns, so its context
	// must not be request-scoped. It carries the HTTP client the Google
	// library fetches tokens with.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenHTTPClient)
	creds, err := google.CredentialsFromJSONWithTypeAndParams(ctx, []byte(key), credType, google.CredentialsParams{
		Scopes: []string{CloudPlatformScope},
	})
	if err != nil {
		// The library's parse errors name fields, not values, but keep the
		// message ours regardless.
		slog.Debug("vertex: credential JSON rejected by the Google library", "type", credType, "error", err)
		return nil, fmt.Errorf("%w: the %s credential JSON could not be loaded", ErrInvalidCredential, credType)
	}
	return oauth2.ReuseTokenSource(nil, creds.TokenSource), nil
}

// AccessToken returns a current access token for a Vertex LLM, minting or
// refreshing one when needed.
func AccessToken(llm *models.LLM) (string, error) {
	ts, err := TokenSource(llm)
	if err != nil {
		return "", err
	}
	tok, err := ts.Token()
	if err != nil {
		// The detail (an oauth2 RetrieveError from a Google endpoint) is
		// logged, not returned: proxy errors reach API clients.
		slog.Warn("vertex: access token request failed", "llm_id", llm.ID, "llm", llm.Name, "error", err)
		return "", fmt.Errorf("%w for LLM %q; check its credential JSON and the service account's permissions", ErrTokenMint, llm.Name)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("%w for LLM %q: empty token", ErrTokenMint, llm.Name)
	}
	return tok.AccessToken, nil
}

// resetTokenSourceCache drops every cached source. Tests only.
func resetTokenSourceCache() {
	cacheMu.Lock()
	cache = map[uint]*cachedSource{}
	cacheMu.Unlock()
}
