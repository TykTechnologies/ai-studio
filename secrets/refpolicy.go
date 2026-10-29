package secrets

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"unicode"
)

// Reference prefixes for values that name a secret or an environment
// variable instead of holding the value.
const (
	SecretRefPrefix = "$SECRET/"
	EnvRefPrefix    = "$ENV/"
)

// EnvRefAllowlistVar is the one operator setting that lists, comma-separated,
// the environment variables a $ENV/NAME reference may resolve. It is empty by
// default, so no $ENV/ reference resolves until an operator lists the name.
// The process environment holds AI Studio's own secrets (encryption key,
// database URL, license), and whoever writes a reference usually chooses
// where its value is sent.
const EnvRefAllowlistVar = "AI_STUDIO_ENV_REF_ALLOWLIST"

// envRefsDisabled is set on edges: they never resolve $ENV/ references; the
// hub resolves what is allowed into the snapshot.
var envRefsDisabled atomic.Bool

// DisableEnvRefs makes ResolveRef refuse every $ENV/ reference in this
// process. Edge gateways call it at startup.
func DisableEnvRefs() { envRefsDisabled.Store(true) }

// EnvRefAllowed reports whether $ENV/name may be resolved: name is listed in
// AI_STUDIO_ENV_REF_ALLOWLIST (and is not that setting itself), and this
// process resolves $ENV/ at all. An empty name, or one containing "/" or
// whitespace, is never allowed, so a bare "$ENV/" (see ForEdge) stays
// unresolved.
func EnvRefAllowed(name string) bool {
	if name == "" || name == EnvRefAllowlistVar || envRefsDisabled.Load() ||
		strings.ContainsRune(name, '/') || strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return false
	}
	for _, allowed := range strings.Split(os.Getenv(EnvRefAllowlistVar), ",") {
		if strings.TrimSpace(allowed) == name {
			return true
		}
	}
	return false
}

// ResolveRef resolves v. It is GetValue(v, false), which applies the policy
// itself: an $ENV/ reference EnvRefAllowed refuses is returned unchanged (see
// IsReference), so the value never leaves the process.
func ResolveRef(v string) string {
	return GetValue(v, false)
}

// IsReference reports whether v is (still) a $SECRET/ or $ENV/ reference,
// e.g. after resolution: an unknown secret or a refused $ENV/ name.
func IsReference(v string) bool {
	return strings.HasPrefix(v, SecretRefPrefix) || strings.HasPrefix(v, EnvRefPrefix)
}

// ForEdge renders a resolved value for a configuration snapshot sent to
// edges. A value that is still a reference is sent as a bare "$ENV/": edges
// released before 2.3 resolve $ENV/ against their own environment, so a
// refused name sent as is would hand them their own variable (an encryption
// key, say) to send on. A bare "$ENV/" resolves to empty there, and current
// edges treat it as an unresolved reference.
func ForEdge(resolved string) string {
	if IsReference(resolved) {
		return EnvRefPrefix
	}
	return resolved
}

// RefusedCredential is what an edge gets for a credential (an API key or
// token) whose reference the hub refused or could not resolve: a value that
// is neither empty nor a reference. Vendor clients fall back to their own
// environment when the key is empty (an OpenAI client reads
// OPENAI_API_KEY, AWS clients their default credential chain), and a
// released edge resolves the bare "$ENV/" ForEdge sends to empty; this
// value is sent instead, as a key, and the provider refuses it.
const RefusedCredential = "refused-credential-reference"

// ForEdgeCredential is ForEdge for a credential field: a value that is still
// a reference, or still encrypted at rest ($ENC/, a value the hub could not
// decrypt), goes out as RefusedCredential. Use ForEdge for fields where an
// empty value has no fallback (endpoints, connection strings).
func ForEdgeCredential(resolved string) string {
	if IsReference(resolved) || strings.HasPrefix(resolved, "$ENC/") {
		return RefusedCredential
	}
	return resolved
}

// ErrUnresolvedCredential is CheckCredential's refusal. It names neither the
// reference nor the value.
var ErrUnresolvedCredential = errors.New("credential is an unresolved reference (a $SECRET/ that cannot be found or decrypted, or an $ENV/ name not listed in " + EnvRefAllowlistVar + ")")

// CheckCredential is called with a resolved credential (an API key or token)
// just before it is sent. It refuses one that did not resolve: still a
// $SECRET/ or $ENV/ reference, still encrypted ($ENC/), or the
// RefusedCredential an edge was sent for one. Such a value is never sent:
// it would hand the destination the reference text. Empty (no credential)
// is allowed.
func CheckCredential(v string) error {
	if v == RefusedCredential || ForEdgeCredential(v) == RefusedCredential {
		return ErrUnresolvedCredential
	}
	return nil
}

// CredentialRefForEdge resolves a credential field for a snapshot sent to
// edges: a reference the policy refuses, that cannot be resolved, or that
// resolves to an empty value goes out as RefusedCredential. An empty
// credential is what a client fills from its own environment, and a secret
// stored empty is never what the writer of the reference meant. A field
// with no value at all (an Ollama LLM without a key) stays empty, and a
// literal value goes out as it is.
func CredentialRefForEdge(ref string) string {
	if ref == "" {
		return ""
	}
	resolved := ResolveRef(ref)
	if resolved == "" {
		return RefusedCredential
	}
	return ForEdgeCredential(resolved)
}

// NewReferences lists the keys of updated whose value is a $SECRET/ or $ENV/
// reference that old does not already hold under the same key (new, changed
// or moved). Writing those needs the right to read secrets, since the value
// is resolved and sent on for the writer.
func NewReferences(old, updated map[string]string) []string {
	var keys []string
	for k, v := range updated {
		if IsReference(v) && old[k] != v {
			keys = append(keys, k)
		}
	}
	return keys
}
