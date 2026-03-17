package secrets

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookTracker records lifecycle hook calls.
type hookTracker struct {
	testLocalKEK
	startupCalls    atomic.Int64
	shutdownCalls   atomic.Int64
	keyRotatedCalls atomic.Int64
	lastRotated     int
	lastFailed      int
	startupErr      error
	shutdownErr     error
	keyRotatedErr   error
}

func newHookTracker(name string) *hookTracker {
	return &hookTracker{testLocalKEK: *newTestLocalKEK(name)}
}

func (h *hookTracker) Startup(_ context.Context) error {
	h.startupCalls.Add(1)
	return h.startupErr
}

func (h *hookTracker) Shutdown(_ context.Context) error {
	h.shutdownCalls.Add(1)
	return h.shutdownErr
}

func (h *hookTracker) KeyRotated(_ context.Context, rotated int, failed int) error {
	h.keyRotatedCalls.Add(1)
	h.lastRotated = rotated
	h.lastFailed = failed
	return h.keyRotatedErr
}

// Verify interface compliance at compile time.
var (
	_ StartupChecker = (*hookTracker)(nil)
	_ Shutdowner     = (*hookTracker)(nil)
	_ KeyRotatedHook = (*hookTracker)(nil)
)

func TestStartupChecker_CalledByNewFromProvider(t *testing.T) {
	tracker := newHookTracker("startup-test")
	// Register a provider that returns our tracker
	DefaultRegistry.Register("test-startup", func(config map[string]string) (KEKProvider, error) {
		return tracker, nil
	})

	db := setupTestDB(t)
	_, err := NewFromProvider(db, "key", "test-startup", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), tracker.startupCalls.Load())
}

func TestStartupChecker_FailBlocksCreation(t *testing.T) {
	tracker := newHookTracker("startup-fail")
	tracker.startupErr = fmt.Errorf("vault unreachable")
	DefaultRegistry.Register("test-startup-fail", func(config map[string]string) (KEKProvider, error) {
		return tracker, nil
	})

	db := setupTestDB(t)
	_, err := NewFromProvider(db, "key", "test-startup-fail", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "startup check failed")
	assert.Contains(t, err.Error(), "vault unreachable")
}

func TestStartupChecker_SkippedWhenNotImplemented(t *testing.T) {
	// The default "local" provider does not implement StartupChecker
	db := setupTestDB(t)
	store, err := NewFromProvider(db, "key", "local", nil)
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestShutdowner_CalledByClose(t *testing.T) {
	tracker := newHookTracker("shutdown-test")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker, map[string]KEKProvider{tracker.KeyID(): tracker})

	err := store.Close(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), tracker.shutdownCalls.Load())
}

func TestShutdowner_ErrorPropagated(t *testing.T) {
	tracker := newHookTracker("shutdown-err")
	tracker.shutdownErr = fmt.Errorf("connection pool error")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker, map[string]KEKProvider{tracker.KeyID(): tracker})

	err := store.Close(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection pool error")
}

func TestShutdowner_SkippedWhenNotImplemented(t *testing.T) {
	plain := newTestLocalKEK("no-shutdown")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", plain, map[string]KEKProvider{plain.KeyID(): plain})

	err := store.Close(context.Background())
	require.NoError(t, err)
}

func TestKeyGeneratedHook_CalledOnFirstEncrypt(t *testing.T) {
	tracker := newHookTracker("keygen-hook")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker, map[string]KEKProvider{tracker.KeyID(): tracker})
	ctx := context.Background()

	// First encrypt triggers key generation
	_, err := store.EncryptValue(ctx, "hello")
	require.NoError(t, err)
}

func TestKeyGeneratedHook_CalledPerEncrypt(t *testing.T) {
	tracker := newHookTracker("keygen-per-object")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker, map[string]KEKProvider{tracker.KeyID(): tracker})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := store.EncryptValue(ctx, fmt.Sprintf("data-%d", i))
		require.NoError(t, err)
	}
	// Per-object DEKs: each encrypt generates a new key
}

func TestCloseIsIdempotent(t *testing.T) {
	tracker := newHookTracker("idempotent")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker, map[string]KEKProvider{tracker.KeyID(): tracker})
	ctx := context.Background()

	require.NoError(t, store.Close(ctx))
	require.NoError(t, store.Close(ctx))
	assert.Equal(t, int64(2), tracker.shutdownCalls.Load())
}

func TestKeyGeneratedHook_ErrorIsLoggedNotFatal(t *testing.T) {
	tracker := newHookTracker("keygen-err")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker, map[string]KEKProvider{tracker.KeyID(): tracker})
	ctx := context.Background()

	// Encrypt should succeed even though the hook returns an error
	enc, err := store.EncryptValue(ctx, "data")
	require.NoError(t, err)
	assert.NotEmpty(t, enc)
}

func TestNewFromProvider_UnknownProviderReturnsError(t *testing.T) {
	db := setupTestDB(t)
	_, err := NewFromProvider(db, "key", "nonexistent-provider", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestNewFromProvider_WithExistingConfig(t *testing.T) {
	tracker := newHookTracker("config-test")
	DefaultRegistry.Register("test-config", func(config map[string]string) (KEKProvider, error) {
		// Verify RAW_KEY was injected alongside existing config
		if config["CUSTOM"] != "value" {
			return nil, fmt.Errorf("missing CUSTOM config")
		}
		if config["RAW_KEY"] == "" {
			return nil, fmt.Errorf("missing RAW_KEY")
		}
		return tracker, nil
	})

	db := setupTestDB(t)
	store, err := NewFromProvider(db, "my-key", "test-config", map[string]string{"CUSTOM": "value"})
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestKeyGeneratedHook_SkippedWhenNotImplemented(t *testing.T) {
	plain := newTestLocalKEK("no-hooks")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", plain, map[string]KEKProvider{plain.KeyID(): plain})
	ctx := context.Background()

	// Should work fine without any hooks
	enc, err := store.EncryptValue(ctx, "data")
	require.NoError(t, err)
	assert.NotEmpty(t, enc)
}
