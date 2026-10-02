package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// Authentication methods (AuthIdentity.Method).
const (
	AuthMethodAppKey = "app_key" // an app secret, as a bearer token or an API key
	AuthMethodOAuth  = "oauth"   // a Studio OAuth access token (MCP)
	AuthMethodPlugin = "plugin"  // an auth plugin
)

// AuthIdentity is who authenticated a request and how. The credential
// validator puts it on the request context for every method; analytics, the
// post-auth hook and the plugins read it.
type AuthIdentity struct {
	AppID  uint
	Method string // AuthMethod*
	// The plugin that authenticated the request (AuthMethodPlugin).
	PluginID   uint
	PluginName string
	// Subject is who the call is for, when an auth plugin says; audit only.
	Subject string
	Claims  map[string]string
	// OAuthUserID is the Studio user an OAuth access token was issued to.
	OAuthUserID uint
}

type authIdentityKey struct{}

// WithAuthIdentity returns ctx carrying id.
func WithAuthIdentity(ctx context.Context, id *AuthIdentity) context.Context {
	return context.WithValue(ctx, authIdentityKey{}, id)
}

// AuthIdentityFromContext returns the identity the credential validator
// recorded, or nil.
func AuthIdentityFromContext(ctx context.Context) *AuthIdentity {
	id, _ := ctx.Value(authIdentityKey{}).(*AuthIdentity)
	return id
}

// The /ai/ -> /llm/call/ loopback hands the outer hop's identity to the inner
// hop. The endpoint the caller addressed (the /ai/ LLM or router) decided how
// the caller authenticates; the inner hop would otherwise authenticate the
// same credential again against the LLM the route picked, whose auth plugins
// may differ from the route's or be none, refusing a caller the route
// accepted (or asking a plugin a second time). Like the failover and router
// markers, the headers are trusted only with this process's token, and are
// stripped before the request leaves for the vendor.
const (
	hdrAuthHandoffApp     = "X-Tyk-Auth-App"
	hdrAuthHandoffMethod  = "X-Tyk-Auth-Method"
	hdrAuthHandoffPlugin  = "X-Tyk-Auth-Plugin"
	hdrAuthHandoffName    = "X-Tyk-Auth-Plugin-Name"
	hdrAuthHandoffSubject = "X-Tyk-Auth-Subject"
	hdrAuthHandoffClaims  = "X-Tyk-Auth-Claims"
)

// authHandoffHeaders adds the hand-off for the identity on ctx to h, and
// reports whether there was one.
func authHandoffHeaders(ctx context.Context, h http.Header) bool {
	id := AuthIdentityFromContext(ctx)
	if id == nil || id.AppID == 0 {
		return false
	}
	h.Set(hdrAuthHandoffApp, strconv.FormatUint(uint64(id.AppID), 10))
	h.Set(hdrAuthHandoffMethod, id.Method)
	h.Set(hdrAuthHandoffPlugin, strconv.FormatUint(uint64(id.PluginID), 10))
	setIfNotEmpty(h, hdrAuthHandoffName, id.PluginName)
	setIfNotEmpty(h, hdrAuthHandoffSubject, id.Subject)
	if len(id.Claims) > 0 {
		if b, err := json.Marshal(id.Claims); err == nil {
			h.Set(hdrAuthHandoffClaims, string(b))
		}
	}
	return true
}

// parseAuthHandoff reads a trusted hand-off off the inner-hop request.
// Anything untrusted is ignored, and the request authenticates as usual.
func (p *Proxy) parseAuthHandoff(r *http.Request) (*AuthIdentity, bool) {
	raw := r.Header.Get(hdrAuthHandoffApp)
	if raw == "" || !p.tokenMatches(r) {
		return nil, false
	}
	appID, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || appID == 0 {
		return nil, false
	}
	pluginID, _ := strconv.ParseUint(r.Header.Get(hdrAuthHandoffPlugin), 10, 32)
	id := &AuthIdentity{
		AppID:      uint(appID),
		Method:     r.Header.Get(hdrAuthHandoffMethod),
		PluginID:   uint(pluginID),
		PluginName: r.Header.Get(hdrAuthHandoffName),
		Subject:    r.Header.Get(hdrAuthHandoffSubject),
	}
	if c := r.Header.Get(hdrAuthHandoffClaims); c != "" {
		_ = json.Unmarshal([]byte(c), &id.Claims)
	}
	return id, true
}

// stripAuthHandoffHeaders removes the hand-off before a request leaves for
// the vendor, and from requests that did not come over the loopback.
func stripAuthHandoffHeaders(h http.Header) {
	h.Del(hdrAuthHandoffApp)
	h.Del(hdrAuthHandoffMethod)
	h.Del(hdrAuthHandoffPlugin)
	h.Del(hdrAuthHandoffName)
	h.Del(hdrAuthHandoffSubject)
	h.Del(hdrAuthHandoffClaims)
}
