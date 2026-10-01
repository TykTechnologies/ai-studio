//go:build !enterprise

package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The studio binary's exit status is what supervisors act on: a server that
// fails to start (a port in use, a missing TLS certificate) must end the
// process with a non-zero status, so Restart=on-failure restarts it; a
// SIGTERM is a clean stop, status 0. These tests run main() in a child
// process (this test binary, re-executed).

const mainChildEnv = "STUDIO_MAIN_TEST_CHILD"

// TestMainChild is main() in the child process; it does nothing otherwise.
func TestMainChild(t *testing.T) {
	if os.Getenv(mainChildEnv) != "1" {
		t.Skip("runs as the child of the exit status tests")
	}
	flag.CommandLine = flag.NewFlagSet("studio", flag.ExitOnError)
	os.Args = []string{"studio", "-env", os.Getenv("STUDIO_MAIN_TEST_ENV_FILE")}
	main()
	os.Exit(0)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startChild runs main() in a child on a fresh SQLite database with extra
// environment, and returns the command and a channel with its exit error.
func startChild(t *testing.T, extra map[string]string) (*exec.Cmd, <-chan error) {
	t.Helper()
	dir := t.TempDir()
	envFile := filepath.Join(dir, "empty.env")
	require.NoError(t, os.WriteFile(envFile, nil, 0o600))

	env := map[string]string{
		mainChildEnv:                "1",
		"STUDIO_MAIN_TEST_ENV_FILE": envFile,
		"PATH":                      os.Getenv("PATH"),
		"HOME":                      dir,
		"DATABASE_TYPE":             "sqlite",
		"DATABASE_URL":              filepath.Join(dir, "studio.db"),
		"SERVER_PORT":               strconv.Itoa(freePort(t)),
		"PROXY_PORT":                strconv.Itoa(freePort(t)),
		"TYK_AI_SECRET_KEY":         "exit-status-test-secret",
		"TELEMETRY_ENABLED":         "false",
		"MARKETPLACE_ENABLED":       "false",
		"DOCS_DISABLED":             "true",
		"EXPORT_STORAGE_PATH":       filepath.Join(dir, "exports"),
		"BRANDING_STORAGE_PATH":     filepath.Join(dir, "branding"),
		"SKIP_FILTER_DEFAULTS":      "true",
		"LOG_LEVEL":                 "warn",
	}
	for k, v := range extra {
		env[k] = v
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainChild$", "-test.v=false")
	cmd.Dir = dir // no .env of the repository
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
	})
	return cmd, done
}

func exitCode(t *testing.T, done <-chan error, within time.Duration) int {
	t.Helper()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		require.NoError(t, err)
		return 0
	case <-time.After(within):
		t.Fatalf("the process did not exit within %s", within)
		return -1
	}
}

// The gRPC control server's port is taken: the process stops and exits 1
// (it exited 0 after a graceful shutdown, so on-failure restarts never ran).
func TestServerStartFailureExitsNonZero(t *testing.T) {
	taken, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer taken.Close()

	_, done := startChild(t, map[string]string{
		"GATEWAY_MODE":                "control",
		"GRPC_PORT":                   strconv.Itoa(taken.Addr().(*net.TCPAddr).Port),
		"GRPC_TLS_INSECURE":           "true",
		"GRPC_AUTH_TOKEN":             "exit-status-test-token",
		"MICROGATEWAY_ENCRYPTION_KEY": "0123456789abcdef0123456789abcdef",
	})
	assert.Equal(t, 1, exitCode(t, done, 2*time.Minute))
}

// SIGTERM on a serving process is a clean stop: status 0.
func TestSIGTERMExitsZero(t *testing.T) {
	port := freePort(t)
	cmd, done := startChild(t, map[string]string{"SERVER_PORT": strconv.Itoa(port)})

	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Minute, 100*time.Millisecond, "Studio serves")

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	assert.Equal(t, 0, exitCode(t, done, time.Minute))
}
