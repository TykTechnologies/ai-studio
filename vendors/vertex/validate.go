package vertexVendor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// Config validation for Vertex LLMs, shared by AI Studio (LLM create and
// update) and the Microgateway (its management API). It never contacts
// Google and never quotes the key.

// ErrKeylessGatewayRoutes is returned by ValidateKey as a warning, not a
// rejection: a key-less Vertex LLM works in Studio chat (host ADC, for now)
// but is refused on gateway routes unless the operator enabled
// AI_STUDIO_VERTEX_ADC.
var ErrKeylessGatewayRoutes = fmt.Errorf("no credential: gateway routes (/llm, /ai, /v1) refuse this Vertex LLM unless the operator sets %s=true", ADCSetting)

// ValidateEndpoint checks a Vertex API endpoint: "project:location" with a
// region (us-central1), a multi-region (us, eu) or global, a domain-scoped
// project allowed (example.com:my-project:us-central1), or an http(s) URL,
// optionally ending in [/v1]/projects/{project}/locations/{location}. The
// rules are the native route's (UpstreamBaseURLFor), so a value accepted
// here is one the proxy can serve.
func ValidateEndpoint(endpoint string) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("%w (it is empty)", ErrInvalidEndpoint)
	}
	_, err := UpstreamBaseURLFor(endpoint)
	return err
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

// ValidateConfig is the check AI Studio and the Microgateway run when a
// Vertex LLM is created or updated. checkEndpoint and checkKey say which
// fields the caller is setting; a key that is a secret reference or an
// encrypted value is resolved later and not checked here. A key-less LLM
// is accepted: warning is then ErrKeylessGatewayRoutes, for the caller to
// log. err is a rejection; neither quotes the key.
func ValidateConfig(endpoint, key string, checkEndpoint, checkKey bool) (warning, err error) {
	if checkEndpoint {
		if err := ValidateEndpoint(endpoint); err != nil {
			return nil, err
		}
	}
	if !checkKey || secrets.CheckCredential(key) != nil {
		return nil, nil
	}
	if err := ValidateKey(key); err != nil {
		if IsWarning(err) {
			return err, nil
		}
		return nil, err
	}
	return nil, nil
}

// IsWarning reports whether a ValidateKey error is only a warning.
func IsWarning(err error) bool {
	return errors.Is(err, ErrKeylessGatewayRoutes)
}
