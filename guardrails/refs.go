package guardrails

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// ResolveConnectionValue resolves one guardrail connection value with the
// platform reference policy (secrets.ResolveRef): $SECRET/ from the secret
// store, $ENV/ only for names in AI_STUDIO_ENV_REF_ALLOWLIST. A value left
// unresolved is still a reference (IsReference), so the provider is never
// built with it and the filter's fail mode applies.
func ResolveConnectionValue(v string) string {
	return secrets.ResolveRef(v)
}

// IsReference reports whether v is still a $SECRET/ or $ENV/ reference.
func IsReference(v string) bool { return secrets.IsReference(v) }

// NewConnectionReferences lists the connection keys of updated (as
// "connection.<key>") holding a $SECRET/ or $ENV/ reference old does not hold
// under the same key. Saving those needs secrets read.
func NewConnectionReferences(old, updated map[string]string) []string {
	keys := secrets.NewReferences(old, updated)
	for i, k := range keys {
		keys[i] = "connection." + k
	}
	return keys
}
