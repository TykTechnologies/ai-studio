// Package oauthscope is the single definition of the OAuth scopes this platform
// issues and enforces.
//
// It exists because the two discovery documents used to disagree - the
// authorization server advertised {openid, profile, email, mcp} while the
// protected resource advertised {mcp, mcp_read, mcp_write} - and nothing
// anywhere read a scope back to make a decision. Both documents now source
// their lists from here, and Grants is the one place a scope is checked.
package oauthscope

import "strings"

// The scopes this platform understands.
const (
	// OpenID, Profile and Email are identity scopes. They say something about
	// the user; they grant no access to a resource.
	OpenID  = "openid"
	Profile = "profile"
	Email   = "email"

	// MCP grants access to the tool endpoints and their MCP transports. It is
	// the only resource scope: mcp_read/mcp_write were advertised but never
	// requested, granted or checked, and honouring them would mean gating
	// individual MCP methods.
	MCP = "mcp"
)

// Supported is what /.well-known/oauth-authorization-server advertises as
// scopes_supported, and what an authorization request is validated against.
var Supported = []string{OpenID, Profile, Email, MCP}

// Resource is what the gateway's /.well-known/oauth-protected-resource
// advertises: the scopes that actually gate access to a resource.
var Resource = []string{MCP}

// Default is granted when a client requests no scope at all, so that a client
// which omits the parameter still ends up with a usable MCP token.
const Default = MCP

// Grants reports whether a token's scope string - space-separated, per RFC 6749
// section 3.3 - includes the required scope.
func Grants(tokenScope, required string) bool {
	for _, s := range strings.Fields(tokenScope) {
		if s == required {
			return true
		}
	}
	return false
}

// IsSupported reports whether a single scope value is one this platform issues.
func IsSupported(scope string) bool {
	for _, s := range Supported {
		if s == scope {
			return true
		}
	}
	return false
}

// Unsupported returns the scopes in a requested scope string that this platform
// does not issue. An empty result means the request is acceptable.
func Unsupported(requested string) []string {
	var bad []string
	for _, s := range strings.Fields(requested) {
		if !IsSupported(s) {
			bad = append(bad, s)
		}
	}
	return bad
}
