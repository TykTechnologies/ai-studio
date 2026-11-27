package vertexVendor

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Config validation for Vertex LLMs, shared by AI Studio (LLM create and
// update) and the Microgateway (its management API). It never contacts
// Google and never quotes the key.

// projectPattern is a Google Cloud project ID, optionally domain-scoped
// (example.com:my-project).
var projectPattern = regexp.MustCompile(`^([a-z0-9-]+(\.[a-z0-9-]+)+:)?[a-z][a-z0-9-]*[a-z0-9]$`)

// ErrKeylessGatewayRoutes is returned by ValidateKey as a warning, not a
// rejection: a key-less Vertex LLM works in Studio chat (host ADC, for now)
// but is refused on gateway routes unless the operator enabled
// AI_STUDIO_VERTEX_ADC.
var ErrKeylessGatewayRoutes = fmt.Errorf("no credential: gateway routes (/llm, /ai, /v1) refuse this Vertex LLM unless the operator sets %s=true", ADCSetting)

// ValidateEndpoint checks a Vertex API endpoint: "project:location" with a
// region (us-central1), a multi-region (us, eu) or global, or an http(s)
// URL.
func ValidateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("%w (it is empty)", ErrInvalidEndpoint)
	}
	if _, err := UpstreamBaseURLFor(endpoint); err != nil {
		return err
	}
	if strings.Contains(endpoint, "://") {
		return nil
	}
	project, _, _ := ParseProjectLocation(endpoint)
	if !projectPattern.MatchString(project) {
		return fmt.Errorf("%w: %q is not a Google Cloud project ID", ErrInvalidEndpoint, project)
	}
	return nil
}

// ValidateKey checks a Vertex LLM's key: a Google credential JSON of a
// supported type, an access token, or empty. An empty key returns
// ErrKeylessGatewayRoutes, which callers treat as a warning. A secret
// reference is not resolvable here; callers skip it.
func ValidateKey(key string) error {
	key = strings.TrimSpace(key)
	switch {
	case key == "":
		return ErrKeylessGatewayRoutes
	case IsAccessToken(key):
		return nil
	case !LooksLikeCredentialJSON(key):
		return fmt.Errorf("%w: the API key must be a Google credential JSON (a service account key or a Workload Identity Federation config) or an access token starting with %q", ErrInvalidCredential, AccessTokenPrefix)
	}
	_, err := ValidateCredentialJSON(key)
	return err
}

// IsWarning reports whether a ValidateKey error is only a warning.
func IsWarning(err error) bool {
	return errors.Is(err, ErrKeylessGatewayRoutes)
}
