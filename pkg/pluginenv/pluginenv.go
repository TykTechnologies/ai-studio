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
)

// EnvPassthrough ("PLUGIN_ENV_PASSTHROUGH") is a comma-separated list of
// extra host variables to pass to plugins. An entry ending in '*' matches
// every variable with that prefix ("GITHUB_RAG_*").
const EnvPassthrough = "PLUGIN_ENV_PASSTHROUGH"

// DefaultAllowlist is what every plugin gets from the host environment:
// process basics, locale, TLS trust, proxies, Go runtime tuning, and the
// non-secret settings the plugin SDK and shipped plugins read (runtime
// detection, edge ID, the internal-network development flag). Entries ending
// in '*' are prefixes.
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
	"EDGE_ID", "ALLOW_INTERNAL_NETWORK_ACCESS",
}

// Environ returns the host variables a plugin may see, in KEY=value form:
// those on DefaultAllowlist and those named by PLUGIN_ENV_PASSTHROUGH.
func Environ() []string {
	allow := append([]string(nil), DefaultAllowlist...)
	for _, name := range strings.Split(os.Getenv(EnvPassthrough), ",") {
		if name = strings.TrimSpace(name); name != "" {
			allow = append(allow, name)
		}
	}

	env := []string{}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if allowed(name, allow) {
			env = append(env, kv)
		}
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

func allowed(name string, allow []string) bool {
	for _, entry := range allow {
		if prefix, ok := strings.CutSuffix(entry, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if name == entry {
			return true
		}
	}
	return false
}
