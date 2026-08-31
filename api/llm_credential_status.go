package api

import (
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// Credential status values reported alongside a provider's api_key.
//
// A brand-new instance bootstraps OPENAI_KEY and ANTHROPIC_KEY as secrets with
// *empty values*, then seeds providers pointing at them with Active: true. The
// list rendered a healthy "Proxied" dot regardless, so the instance looked
// completely configured and the first real call failed with nothing in the UI
// explaining why. These let the UI tell the three cases apart.
const (
	// CredentialUnset means no api_key is configured at all.
	CredentialUnset = "unset"
	// CredentialInline means a literal key is stored on the provider.
	CredentialInline = "inline"
	// CredentialSecret means the key is a $SECRET/ or $ENV/ reference that
	// resolves to a non-empty value.
	CredentialSecret = "secret"
	// CredentialUnresolved means the key is a $SECRET/ or $ENV/ reference that
	// resolves to nothing -- the dangling reference that makes a fresh instance
	// look configured when it is not.
	CredentialUnresolved = "unresolved"
)

// credentialStatus classifies a provider's stored api_key, and names the
// referenced secret when there is one so the UI can say which one to fill in.
func credentialStatus(apiKey string) (status string, reference string) {
	if apiKey == "" {
		return CredentialUnset, ""
	}

	if !strings.HasPrefix(apiKey, "$SECRET/") && !strings.HasPrefix(apiKey, "$ENV/") {
		return CredentialInline, ""
	}

	parts := strings.SplitN(apiKey, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return CredentialUnresolved, ""
	}
	name := parts[1]

	// preserveRef=false so we get the resolved value rather than the reference.
	if resolved := secrets.GetValue(apiKey, false); resolved != "" && resolved != apiKey {
		return CredentialSecret, name
	}
	return CredentialUnresolved, name
}
