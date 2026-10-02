package proxy

import "net/http"

// AuthHooks provides extension points in the authentication lifecycle.
// These hooks are for LLM proxy requests, NOT for OAuth/MCP flows.
// All hooks are optional (nil checks are performed before calling).
type AuthHooks struct {
	// PreAuth runs BEFORE credential extraction and validation.
	// Only called for non-OAuth requests (LLM proxy requests).
	// Has NO access to authenticated user/app data.
	// Return true to block the request.
	PreAuth func(w http.ResponseWriter, r *http.Request) bool

	// CustomAuth lets auth plugins authenticate a request in place of app
	// keys. It is called with the endpoint the request addresses (an LLM,
	// datasource, tool, router or plugin endpoint) and the credential the
	// request presented, for every request that is not a Studio OAuth access
	// token.
	//
	// An endpoint with auth plugins attached is authenticated by them alone:
	// the hook answers AuthAccepted or AuthRejected, and app keys are not
	// tried. An endpoint without any gets AuthNotHandled, and app keys apply
	// as usual. An error means the plugins could not be asked (none could be
	// loaded, say); the request is refused with a 503.
	CustomAuth func(r *http.Request, target AuthTarget, credential, credType string) (AuthResult, error)

	// PostAuth runs AFTER successful authentication (standard or custom).
	// Only called for non-OAuth requests (LLM proxy requests).
	// Receives the authenticated app ID.
	// Return true to block the request.
	PostAuth func(w http.ResponseWriter, r *http.Request, appID uint) bool
}

// Kinds of endpoint an auth plugin can be attached to (AuthTarget.Kind).
const (
	AuthTargetLLM            = "llm"
	AuthTargetDatasource     = "datasource"
	AuthTargetTool           = "tool"
	AuthTargetModelRouter    = "model_router"
	AuthTargetSemanticRouter = "semantic_router"
	AuthTargetPlugin         = "plugin" // a custom_endpoint plugin's /plugins/{slug}/ routes
)

// AuthTarget is the endpoint a request addresses, resolved from its path.
type AuthTarget struct {
	Kind string // AuthTarget*
	ID   uint
	Slug string
}

// Credential types passed to CustomAuth.
const (
	CredTypeBearer = "bearer"  // Authorization: Bearer ...
	CredTypeAPIKey = "api_key" // a vendor key header, x-api-key, a bare Authorization value or ?apiKey=
)

// AuthOutcome is what the auth plugins decided.
type AuthOutcome int

const (
	// AuthNotHandled: the endpoint has no auth plugins; app keys apply.
	AuthNotHandled AuthOutcome = iota
	// AuthAccepted: a plugin authenticated the request as AuthResult.AppID.
	AuthAccepted
	// AuthRejected: the endpoint's plugins all refused the credential.
	AuthRejected
)

// AuthResult is the outcome of CustomAuth.
type AuthResult struct {
	Outcome AuthOutcome
	AppID   uint
	// Subject is who the call is for, when the credential says (the auth
	// response's user id: a delegated token's sub, say). For audit only.
	Subject string
	// Claims the plugin returned alongside, for audit and post-auth plugins.
	Claims     map[string]string
	PluginID   uint
	PluginName string
	// Reason is why the plugins refused, for the log. It is not sent to the
	// caller.
	Reason string
}
