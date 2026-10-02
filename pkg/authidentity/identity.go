// Package authidentity carries who authenticated a gateway request, and how,
// on the request context. The proxy's credential validator records it; the
// analytics writers, the post-auth hook and the plugins read it. It is a leaf
// package so analytics can read it without importing the proxy.
package authidentity

import "context"

// Authentication methods (Identity.Method).
const (
	MethodAppKey = "app_key" // an app secret, as a bearer token or an API key
	MethodOAuth  = "oauth"   // a Studio OAuth access token (MCP)
	MethodPlugin = "plugin"  // an auth plugin
)

// ClaimActor is the claim an auth plugin sets to the agent acting for the
// subject (a delegated token's act or azp). It is recorded with the request
// as the acting agent.
const ClaimActor = "auth_actor"

// Identity is who authenticated a request and how.
type Identity struct {
	AppID  uint
	Method string // Method*
	// The plugin that authenticated the request (MethodPlugin).
	PluginID   uint
	PluginName string
	// Subject is who the call is for, when an auth plugin says (its user
	// id). It is recorded for audit and never used for authorisation.
	Subject string
	Claims  map[string]string
	// OAuthUserID is the Studio user an OAuth access token was issued to.
	OAuthUserID uint
}

// OnBehalfOf is the subject the request was made for, as recorded in
// analytics; empty unless an auth plugin named one.
func (id *Identity) OnBehalfOf() string {
	if id == nil {
		return ""
	}
	return id.Subject
}

// ActingAgent is the agent acting for the subject, as recorded in analytics;
// empty unless an auth plugin named one (ClaimActor).
func (id *Identity) ActingAgent() string {
	if id == nil {
		return ""
	}
	return id.Claims[ClaimActor]
}

type key struct{}

// With returns ctx carrying id.
func With(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From returns the identity recorded on ctx, or nil.
func From(ctx context.Context) *Identity {
	if ctx == nil {
		return nil
	}
	id, _ := ctx.Value(key{}).(*Identity)
	return id
}
