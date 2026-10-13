// Package pluginenv builds the environment plugin processes start with.
//
// Plugins run as separate processes (HashiCorp go-plugin). By default
// go-plugin hands a plugin the host's whole environment, which includes the
// host's own configuration and credentials. Plugin hosts (AI Studio and the
// microgateway) instead start plugins with SkipHostEnv and a Command from this
// package, so a plugin sees only the variables on DefaultAllowlist plus any the
// operator names in PLUGIN_ENV_PASSTHROUGH. go-plugin adds its own handshake
// variables on top.
package pluginenv

import (
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// EnvPassthrough ("PLUGIN_ENV_PASSTHROUGH") is a comma-separated list of
// extra host variables to pass to plugins. An entry ending in '*' matches
// every variable with that prefix ("GITHUB_RAG_*").
const EnvPassthrough = "PLUGIN_ENV_PASSTHROUGH"

// DefaultAllowlist is what every plugin gets from the host environment:
// process basics, locale, TLS trust, proxies, Go runtime tuning, and the
// non-secret settings the plugin SDK and shipped plugins read (runtime
// detection, edge ID). Entries ending in '*' are prefixes.
//
// ALLOW_INTERNAL_NETWORK_ACCESS is deliberately left out: it relaxes the
// host's own checks, and a plugin should not inherit that relaxation unless
// the operator passes it through.
var DefaultAllowlist = []string{
	// Process basics and locale.
	"PATH", "HOME", "USER", "TMPDIR", "TZ", "LANG", "LC_*",
	// TLS trust store and outbound proxies.
	"SSL_CERT_FILE", "SSL_CERT_DIR",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	// Go runtime tuning.
	"GOMAXPROCS", "GOMEMLIMIT", "GOGC", "GODEBUG", "GOTRACEBACK",
	// Read by the plugin SDK's runtime detection.
	"PLUGIN_RUNTIME", "GATEWAY_MODE", "MICROGATEWAY_MODE",
	// Read by shipped plugins.
	"EDGE_ID",
}

// hostSecrets are host settings a plugin should not normally see. Passing one
// through is allowed, but logged.
var hostSecrets = []string{
	"ENCRYPTION_KEY", "MICROGATEWAY_ENCRYPTION_KEY", "DATABASE_URL",
	"TYK_AI_SECRET_KEY", "TYK_AI_LICENSE", "EDGE_AUTH_TOKEN", "GRPC_AUTH_TOKEN",
	"JWT_SECRET", "*_API_KEY",
}

// Environ returns the host variables a plugin may see, in KEY=value form:
// those on DefaultAllowlist and those named by PLUGIN_ENV_PASSTHROUGH. A
// pass-through entry that is only "*" is ignored, since it would pass the
// whole host environment; a pass-through that lets a host secret through is
// logged.
func Environ() []string {
	var passthrough []string
	for _, name := range strings.Split(os.Getenv(EnvPassthrough), ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "":
		case "*":
			log.Warn().Msgf("Ignoring %q in %s: it would pass the whole host environment to plugins", name, EnvPassthrough)
		default:
			passthrough = append(passthrough, name)
		}
	}

	env := []string{}
	var passedSecrets []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case matches(name, DefaultAllowlist):
			env = append(env, kv)
		case matches(name, passthrough):
			env = append(env, kv)
			if matches(name, hostSecrets) {
				passedSecrets = append(passedSecrets, name)
			}
		}
	}
	if len(passedSecrets) > 0 {
		log.Warn().
			Strs("variables", passedSecrets).
			Msgf("%s passes host secrets to every plugin", EnvPassthrough)
	}
	return env
}

// Command returns an exec.Cmd for a plugin binary whose environment is
// Environ(). Pair it with go-plugin's ClientConfig.SkipHostEnv, or go-plugin
// appends the host environment anyway.
func Command(path string) *exec.Cmd {
	cmd := exec.Command(path)
	cmd.Env = Environ()
	return cmd
}

// matches reports whether name is in list. An entry ending in '*' matches
// that prefix; one starting with '*' matches that suffix.
func matches(name string, list []string) bool {
	for _, entry := range list {
		if prefix, ok := strings.CutSuffix(entry, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if suffix, ok := strings.CutPrefix(entry, "*"); ok {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		} else if name == entry {
			return true
		}
	}
	return false
}
