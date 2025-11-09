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

	// CustomAuth allows plugins to replace the validation step.
	// Only called for non-OAuth requests.
	// Receives the extracted credential string.
	// Returns (appID, authenticated, error):
	//   - appID: The authenticated app ID if authentication succeeds
	//   - authenticated: true if authentication succeeded, false to fall back to standard validation
	//   - error: Any error that occurred during authentication
	// If this is nil or returns (0, false, nil), standard validation is used.
	CustomAuth func(credential string, r *http.Request) (appID uint, authenticated bool, err error)

	// PostAuth runs AFTER successful authentication (standard or custom).
	// Only called for non-OAuth requests (LLM proxy requests).
	// Receives the authenticated app ID.
	// Return true to block the request.
	PostAuth func(w http.ResponseWriter, r *http.Request, appID uint) bool
}
