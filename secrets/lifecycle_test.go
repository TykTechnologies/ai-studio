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
	startupCalls      atomic.Int64
	shutdownCalls     atomic.Int64
	keyGeneratedCalls atomic.Int64
	keyGeneratedIDs   []uint
	keyRotatedCalls   atomic.Int64
	lastRotated       int
	lastFailed        int
	keyRetiredCalls   atomic.Int64
	keyRetiredIDs     []uint
	startupErr        error
	shutdownErr       error
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

func (h *hookTracker) KeyGenerated(_ context.Context, keyID uint) error {
	h.keyGeneratedCalls.Add(1)
	h.keyGeneratedIDs = append(h.keyGeneratedIDs, keyID)
	return nil
}

func (h *hookTracker) KeyRotated(_ context.Context, rotated int, failed int) error {
	h.keyRotatedCalls.Add(1)
	h.lastRotated = rotated
	h.lastFailed = failed
	return nil
}

func (h *hookTracker) KeyRetired(_ context.Context, keyID uint) error {
	h.keyRetiredCalls.Add(1)
	h.keyRetiredIDs = append(h.keyRetiredIDs, keyID)
	return nil
}

// Verify interface compliance at compile time.
var (
	_ StartupChecker  = (*hookTracker)(nil)
	_ Shutdowner      = (*hookTracker)(nil)
	_ KeyGeneratedHook = (*hookTracker)(nil)
	_ KeyRotatedHook   = (*hookTracker)(nil)
	_ KeyRetiredHook   = (*hookTracker)(nil)
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
	store := NewWithKEKProvider(db, "key", tracker)

	err := store.Close(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), tracker.shutdownCalls.Load())
}

func TestShutdowner_ErrorPropagated(t *testing.T) {
	tracker := newHookTracker("shutdown-err")
	tracker.shutdownErr = fmt.Errorf("connection pool error")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker)

	err := store.Close(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection pool error")
}

func TestShutdowner_SkippedWhenNotImplemented(t *testing.T) {
	plain := newTestLocalKEK("no-shutdown")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", plain)

	err := store.Close(context.Background())
	require.NoError(t, err)
}

func TestKeyGeneratedHook_CalledOnFirstEncrypt(t *testing.T) {
	tracker := newHookTracker("keygen-hook")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker)
	ctx := context.Background()

	// First encrypt triggers key generation
	_, err := store.EncryptValue(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, int64(1), tracker.keyGeneratedCalls.Load())
	assert.Len(t, tracker.keyGeneratedIDs, 1)
	assert.Equal(t, uint(1), tracker.keyGeneratedIDs[0])
}

func TestKeyGeneratedHook_NotCalledOnSubsequentEncrypt(t *testing.T) {
	tracker := newHookTracker("keygen-once")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := store.EncryptValue(ctx, fmt.Sprintf("data-%d", i))
		require.NoError(t, err)
	}
	// Key is generated once, then cached
	assert.Equal(t, int64(1), tracker.keyGeneratedCalls.Load())
}

func TestKeyRotatedHook_CalledAfterRotateKEK(t *testing.T) {
	oldKEK := newHookTracker("old-kek")
	newKEK := newHookTracker("new-kek")

	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", oldKEK)
	ctx := context.Background()

	// Create some encrypted data (generates a key)
	_, err := store.EncryptValue(ctx, "data")
	require.NoError(t, err)

	result, err := store.RotateKEK(ctx, oldKEK, newKEK)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Rotated)

	// Hook should be called on the NEW kek
	assert.Equal(t, int64(1), newKEK.keyRotatedCalls.Load())
	assert.Equal(t, 1, newKEK.lastRotated)
	assert.Equal(t, 0, newKEK.lastFailed)

	// Old KEK should NOT get the hook
	assert.Equal(t, int64(0), oldKEK.keyRotatedCalls.Load())
}

func TestKeyRotatedHook_SkippedWhenNotImplemented(t *testing.T) {
	oldKEK := newTestLocalKEK("old")
	newKEK := newTestLocalKEK("new") // plain testLocalKEK, no hooks
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", oldKEK)
	ctx := context.Background()

	_, err := store.EncryptValue(ctx, "data")
	require.NoError(t, err)

	// Should not panic
	_, err = store.RotateKEK(ctx, oldKEK, newKEK)
	require.NoError(t, err)
}

func TestCloseIsIdempotent(t *testing.T) {
	tracker := newHookTracker("idempotent")
	db := setupTestDB(t)
	store := NewWithKEKProvider(db, "key", tracker)
	ctx := context.Background()

	require.NoError(t, store.Close(ctx))
	require.NoError(t, store.Close(ctx))
	assert.Equal(t, int64(2), tracker.shutdownCalls.Load())
}
