//go:build linux || darwin

package ociplugins

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundedCall runs fn with a deadline, failing the test if it does not
// return in time (an unbounded read or a blocking open).
func boundedCall(t *testing.T, name string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not return within 2s: unbounded read or blocking open", name)
		return nil
	}
}

// makeFIFO creates a named pipe and, at cleanup, opens it for writing so a
// reader blocked on it (if the code under test hangs) is released.
func makeFIFO(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.fifo")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	t.Cleanup(func() {
		if f, err := os.OpenFile(path, os.O_RDWR, 0); err == nil {
			f.Close()
		}
	})
	return path
}

// A public key ID comes from the plugin's ?pubkey= parameter (an admin, or
// the hub for gateways). Naming a device, a FIFO or a huge file must fail
// fast, not read forever or block (midsommar-clean-c1g.21).
func TestResolvePublicKey_RejectsNonRegularAndOversizedFiles(t *testing.T) {
	fifo := makeFIFO(t)

	big := filepath.Join(t.TempDir(), "big.pem")
	// A valid key padded past 64 KiB: only a size limit rejects it.
	require.NoError(t, os.WriteFile(big, []byte(testPublicKeyPEM+strings.Repeat("\n", 70*1024)), 0o600))

	good := filepath.Join(t.TempDir(), "good.pem")
	require.NoError(t, os.WriteFile(good, []byte(testPublicKeyPEM), 0o600))

	cfg := &OCIConfig{}
	for _, keyID := range []string{"/dev/zero", "file:/dev/zero", "/dev/urandom", fifo, "file:" + fifo, big, "file:" + big} {
		t.Run(keyID, func(t *testing.T) {
			err := boundedCall(t, "ResolvePublicKey", func() error {
				_, err := ResolvePublicKey(cfg, keyID)
				return err
			})
			assert.Error(t, err)
		})
	}

	t.Run("regular key file still resolves", func(t *testing.T) {
		for _, keyID := range []string{good, "file:" + good} {
			var key []byte
			err := boundedCall(t, "ResolvePublicKey", func() error {
				var err error
				key, err = ResolvePublicKey(cfg, keyID)
				return err
			})
			require.NoError(t, err, keyID)
			assert.Contains(t, string(key), "BEGIN PUBLIC KEY", keyID)
		}
	})
}

// A named key file from the environment (OCI_PLUGINS_PUBKEY_FILE_<NAME>)
// gets the same bound.
func TestResolvePublicKey_NamedFileEnvIsBounded(t *testing.T) {
	fifo := makeFIFO(t)
	t.Setenv("OCI_PLUGINS_PUBKEY_FILE_FIFO", fifo)
	t.Setenv("OCI_PLUGINS_PUBKEY_FILE_ZERO", "/dev/zero")
	cfg := &OCIConfig{DefaultPublicKeys: LoadPublicKeysFromEnv()}
	for _, keyID := range []string{"FIFO", "ZERO"} {
		t.Run(keyID, func(t *testing.T) {
			err := boundedCall(t, "ResolvePublicKey", func() error {
				_, err := ResolvePublicKey(cfg, keyID)
				return err
			})
			assert.Error(t, err)
		})
	}
}

// The trusted-root override has the same shape and gets the same bound.
func TestValidateTrustedRootFile_RejectsNonRegularFiles(t *testing.T) {
	fifo := makeFIFO(t)
	for _, path := range []string{"/dev/zero", fifo} {
		t.Run(path, func(t *testing.T) {
			err := boundedCall(t, "ValidateTrustedRootFile", func() error {
				return ValidateTrustedRootFile(path)
			})
			assert.Error(t, err)
		})
	}
}
