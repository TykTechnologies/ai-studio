package guardrails

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// A guardrail classifier on the same host can be reached over a Unix domain
// socket (an HTTP classifier endpoint unix:///run/guard/guard.sock), so it
// needs no TCP port and can run with no network at all.
//
// A socket endpoint is an HTTP request to a local socket chosen by whoever
// may write filters, and local sockets include admin APIs
// (/var/run/docker.sock). So a socket is refused unless the operator lists
// it in EnvUnixSocketAllowlist: exact socket paths, or directories (ending
// in "/") whose sockets are allowed. The check runs when a filter is saved
// and again before every dial, on every node, after resolving symlinks: the
// path as written and the path it resolves to must both be allowed, and
// what it resolves to must be a socket.

// EnvUnixSocketAllowlist is the comma-separated list of allowed socket
// paths and directories ("/run/guard/", with the trailing slash). Empty by
// default: no socket endpoint is allowed.
const EnvUnixSocketAllowlist = "GUARDRAIL_UNIX_SOCKET_ALLOWLIST"

// UnixScheme is the scheme of a socket endpoint.
const UnixScheme = "unix"

// ErrUnixSocketRefused is a socket endpoint the allowlist does not cover, or
// that is not a socket.
var ErrUnixSocketRefused = errors.New("guardrail unix socket refused")

// UnixSocketPath reads a socket endpoint: unix:///run/guard/guard.sock is
// the socket /run/guard/guard.sock. ok is false for any other endpoint. A
// unix endpoint with a host, a query or a relative path is an error.
func UnixSocketPath(endpoint string) (path string, ok bool, err error) {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.HasPrefix(strings.ToLower(endpoint), UnixScheme+":") {
		return "", false, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", true, fmt.Errorf("%w: invalid unix endpoint", ErrUnixSocketRefused)
	}
	if u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || !filepath.IsAbs(u.Path) {
		return "", true, fmt.Errorf("%w: a unix endpoint is unix:///absolute/path/to.sock (the request path goes in the path connection field)", ErrUnixSocketRefused)
	}
	return filepath.Clean(u.Path), true, nil
}

// unixSocketAllowlist is the operator's allowlist, each entry cleaned, both
// as written and with its symlinks resolved where it exists (so an endpoint
// written the way the entry was, /tmp/... where /tmp is a symlink, matches
// as written and once resolved); a directory entry keeps its trailing
// slash.
func unixSocketAllowlist() []string {
	var out []string
	for _, e := range strings.Split(os.Getenv(EnvUnixSocketAllowlist), ",") {
		e = strings.TrimSpace(e)
		if e == "" || !filepath.IsAbs(e) {
			continue
		}
		dir := strings.HasSuffix(e, "/")
		forms := []string{filepath.Clean(e)}
		if r, err := filepath.EvalSymlinks(forms[0]); err == nil && r != forms[0] {
			forms = append(forms, r)
		}
		for _, p := range forms {
			if dir {
				p = strings.TrimSuffix(p, "/") + "/"
			}
			out = append(out, p)
		}
	}
	return out
}

func socketAllowed(path string, allow []string) bool {
	for _, a := range allow {
		if strings.HasSuffix(a, "/") {
			if strings.HasPrefix(path, a) && len(path) > len(a) {
				return true
			}
			continue
		}
		if path == a {
			return true
		}
	}
	return false
}

// CheckUnixSocket checks a socket path against the allowlist and returns
// the path to dial: the path with its symlinks resolved. Both the path as
// written and the resolved one must be allowed. With mustExist (at dial
// time) the path must exist and be a socket; without it (when a filter is
// saved on a hub that may not mount an edge's socket) a missing path is
// checked as written, with its directory's symlinks resolved, and an
// existing one must still be a socket.
func CheckUnixSocket(path string, mustExist bool) (string, error) {
	allow := unixSocketAllowlist()
	if len(allow) == 0 {
		return "", fmt.Errorf("%w: socket endpoints are not enabled (%s is empty)", ErrUnixSocketRefused, EnvUnixSocketAllowlist)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: the socket path must be absolute", ErrUnixSocketRefused)
	}
	clean := filepath.Clean(path)
	if !socketAllowed(clean, allow) {
		return "", fmt.Errorf("%w: %s is not in %s", ErrUnixSocketRefused, clean, EnvUnixSocketAllowlist)
	}
	resolved, err := filepath.EvalSymlinks(clean)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist) && !mustExist:
		dir, derr := filepath.EvalSymlinks(filepath.Dir(clean))
		if derr != nil {
			dir = filepath.Dir(clean)
		}
		resolved = filepath.Join(dir, filepath.Base(clean))
	case errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("guardrail unix socket %s does not exist", clean)
	default:
		return "", fmt.Errorf("%w: %s cannot be resolved", ErrUnixSocketRefused, clean)
	}
	if !socketAllowed(resolved, allow) {
		return "", fmt.Errorf("%w: %s resolves outside %s", ErrUnixSocketRefused, clean, EnvUnixSocketAllowlist)
	}
	info, err := os.Lstat(resolved)
	switch {
	case err == nil && info.Mode()&os.ModeSocket == 0:
		return "", fmt.Errorf("%w: %s is not a socket", ErrUnixSocketRefused, clean)
	case err != nil && mustExist:
		return "", fmt.Errorf("guardrail unix socket %s is not reachable", clean)
	}
	return resolved, nil
}

// CheckEndpoint is the check a config's endpoint must pass when a filter is
// saved or tested: a literal socket endpoint must be allowed. It is not part
// of Normalize, which also runs where a filter runs: there the check is made
// at dial time, so a socket a node does not allow is a provider error and
// the fail mode and unscanned rules apply, as for an unreachable provider.
func CheckEndpoint(cfg Config) error {
	if cfg.Provider != ProviderHTTP {
		return nil
	}
	return checkUnixEndpoint(cfg.Connection["endpoint"])
}

// checkUnixEndpoint validates a literal socket endpoint. A $SECRET/ or
// $ENV/ reference is checked at dial time, once resolved.
func checkUnixEndpoint(endpoint string) error {
	if IsReference(endpoint) {
		return nil
	}
	path, ok, err := UnixSocketPath(endpoint)
	if !ok {
		return nil
	}
	if err == nil {
		_, err = CheckUnixSocket(path, false)
	}
	if err != nil {
		return fmt.Errorf("%w: connection.endpoint: %w", ErrInvalidConfig, err)
	}
	return nil
}
