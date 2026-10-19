package guardrails

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// socketDir makes a short directory for sockets (macOS caps a socket path at
// 104 bytes, which t.TempDir exceeds) and returns it with its symlinks
// resolved (/tmp is /private/tmp there).
func socketDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "gs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	r, err := filepath.EvalSymlinks(d)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func listen(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
}

func TestUnixSocketPath(t *testing.T) {
	cases := map[string]struct {
		path    string
		ok, bad bool
	}{
		"unix:///run/guard/guard.sock":       {path: "/run/guard/guard.sock", ok: true},
		"UNIX:///run/guard/./x/../guard.sock": {path: "/run/guard/guard.sock", ok: true},
		"https://classifier/v1":              {},
		"unix://host/run/guard.sock":         {ok: true, bad: true},
		"unix:relative.sock":                 {ok: true, bad: true},
		"unix:///run/guard.sock?x=1":         {ok: true, bad: true},
	}
	for in, want := range cases {
		path, ok, err := UnixSocketPath(in)
		if ok != want.ok || (err != nil) != want.bad || (!want.bad && path != want.path) {
			t.Errorf("%s: path %q ok %v err %v", in, path, ok, err)
		}
	}
}

// Nothing is allowed until the operator lists it; then exactly what is
// listed, a directory's sockets, never a path that resolves elsewhere, and
// never something that is not a socket.
func TestCheckUnixSocket(t *testing.T) {
	dir := socketDir(t)
	allowed := filepath.Join(dir, "allowed")
	outside := filepath.Join(dir, "outside")
	for _, d := range []string{allowed, outside} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	guard := filepath.Join(allowed, "guard.sock")
	listen(t, guard)
	admin := filepath.Join(outside, "docker.sock") // stands in for an admin API
	listen(t, admin)
	if err := os.Symlink(admin, filepath.Join(allowed, "escape.sock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(guard, filepath.Join(outside, "into.sock")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(allowed, "file.sock"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	refused := func(path string, mustExist bool) {
		t.Helper()
		if _, err := CheckUnixSocket(path, mustExist); !errors.Is(err, ErrUnixSocketRefused) {
			t.Errorf("%s (mustExist %v): err = %v, want refused", path, mustExist, err)
		}
	}

	t.Setenv(EnvUnixSocketAllowlist, "")
	refused(guard, true)

	t.Setenv(EnvUnixSocketAllowlist, guard)
	if got, err := CheckUnixSocket(guard, true); err != nil || got != guard {
		t.Errorf("exact entry: %q %v", got, err)
	}
	refused(filepath.Join(allowed, "other.sock"), false)

	t.Setenv(EnvUnixSocketAllowlist, " /nowhere/, "+allowed+"/ ")
	if _, err := CheckUnixSocket(guard, true); err != nil {
		t.Errorf("directory entry: %v", err)
	}
	refused(filepath.Join(allowed, "escape.sock"), true) // a symlink out of the directory
	refused(filepath.Join(outside, "into.sock"), true)   // a path outside that links in
	refused(filepath.Join(allowed, "file.sock"), true)   // not a socket
	refused(filepath.Join(allowed, "file.sock"), false)
	refused(allowed+"/../outside/docker.sock", true) // dot segments are cleaned first
	refused(admin, true)
	refused("relative.sock", false)

	// A socket the hub does not mount (an edge's) is accepted at save time
	// when its path is allowed, and refused at dial time while missing.
	missing := filepath.Join(allowed, "edge.sock")
	if _, err := CheckUnixSocket(missing, false); err != nil {
		t.Errorf("missing allowed socket at save: %v", err)
	}
	if _, err := CheckUnixSocket(missing, true); err == nil {
		t.Error("missing socket accepted at dial time")
	}
	// The allowlist is resolved too: /tmp/... on a host where /tmp is a
	// symlink still covers the socket.
	t.Setenv(EnvUnixSocketAllowlist, strings.Replace(allowed, "/private/tmp/", "/tmp/", 1)+"/")
	if _, err := CheckUnixSocket(guard, true); err != nil {
		t.Errorf("allowlist entry through a symlink: %v", err)
	}
}

// A filter is refused when it is saved (or tested, or prepared) with a
// socket endpoint the allowlist does not cover; a reference is checked once
// resolved, at dial time.
func TestNormalizeUnixEndpoint(t *testing.T) {
	dir := socketDir(t)
	sock := filepath.Join(dir, "g.sock")
	listen(t, sock)
	cfg := func(endpoint string) Config {
		return Config{Provider: ProviderHTTP, Detectors: []DetectorConfig{{Name: "x"}}, OnDetect: ActionLog, Connection: map[string]string{"endpoint": endpoint}}
	}
	t.Setenv(EnvUnixSocketAllowlist, "")
	if err := CheckEndpoint(cfg("unix://"+sock)); !errors.Is(err, ErrInvalidConfig) || !errors.Is(err, ErrUnixSocketRefused) {
		t.Errorf("no allowlist: err = %v", err)
	}
	if err := CheckEndpoint(cfg("$SECRET/GUARD_ENDPOINT")); err != nil {
		t.Errorf("a reference is checked at dial time: %v", err)
	}
	if err := CheckEndpoint(cfg("unix:///var/run/docker.sock")); err == nil {
		t.Error("docker.sock accepted with no allowlist")
	}
	t.Setenv(EnvUnixSocketAllowlist, dir+"/")
	if err := CheckEndpoint(cfg("unix://"+sock)); err != nil {
		t.Errorf("allowed socket: %v", err)
	}
	if err := CheckEndpoint(cfg("unix:///var/run/docker.sock")); !errors.Is(err, ErrUnixSocketRefused) {
		t.Errorf("docker.sock outside the allowlist: err = %v", err)
	}
}
