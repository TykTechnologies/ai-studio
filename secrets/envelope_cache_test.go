package secrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers for cache tests ---

// countingKeyStore wraps a memKeyStore and counts calls to GetActiveKey and GetKeyByID.
type countingKeyStore struct {
	inner          *memKeyStore
	activeKeyHits  atomic.Int64
	getByIDHits    atomic.Int64
}

func newCountingKeyStore() *countingKeyStore {
	return &countingKeyStore{inner: newMemKeyStore()}
}

func (c *countingKeyStore) GetActiveKey(ctx context.Context) (*EncryptionKey, error) {
	c.activeKeyHits.Add(1)
	return c.inner.GetActiveKey(ctx)
}

func (c *countingKeyStore) GetKeyByID(ctx context.Context, id uint) (*EncryptionKey, error) {
	c.getByIDHits.Add(1)
	return c.inner.GetKeyByID(ctx, id)
}

func (c *countingKeyStore) CreateKey(ctx context.Context, wrappedKey string, status string) (*EncryptionKey, error) {
	return c.inner.CreateKey(ctx, wrappedKey, status)
}

func (c *countingKeyStore) ListKeys(ctx context.Context) ([]EncryptionKey, error) {
	return c.inner.ListKeys(ctx)
}

func (c *countingKeyStore) UpdateKey(ctx context.Context, key *EncryptionKey) error {
	return c.inner.UpdateKey(ctx, key)
}

func seedActiveKeyOnCounting(t *testing.T, ks *countingKeyStore, kek KEKProvider) {
	t.Helper()
	ctx := context.Background()
	dek := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, dek)
	require.NoError(t, err)
	wrapped, err := kek.WrapKey(ctx, dek)
	require.NoError(t, err)
	_, err = ks.CreateKey(ctx, base64.URLEncoding.EncodeToString(wrapped), EncryptionKeyActive)
	require.NoError(t, err)
}

// failingKeyStore returns errors from GetActiveKey and GetKeyByID.
type failingKeyStore struct {
	memKeyStore
	failActive bool
	failGetByID bool
}

func (f *failingKeyStore) GetActiveKey(_ context.Context) (*EncryptionKey, error) {
	if f.failActive {
		return nil, fmt.Errorf("transient db error")
	}
	return f.memKeyStore.GetActiveKey(context.Background())
}

func (f *failingKeyStore) GetKeyByID(_ context.Context, id uint) (*EncryptionKey, error) {
	if f.failGetByID {
		return nil, fmt.Errorf("key not found")
	}
	return f.memKeyStore.GetKeyByID(context.Background(), id)
}

// --- 1. DEK cache tests ---

func TestCache_EncryptDecryptOnlyHitsKeyStoreOnce(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("cache-kek")
	ks := newCountingKeyStore()
	seedActiveKeyOnCounting(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Encrypt 100 times — should only hit GetActiveKey once
	for i := 0; i < 100; i++ {
		_, err := c.Encrypt(ctx, nil, []byte(fmt.Sprintf("data-%d", i)))
		require.NoError(t, err)
	}
	assert.Equal(t, int64(1), ks.activeKeyHits.Load(), "GetActiveKey should be called exactly once due to caching")

	// Decrypt all of them — GetKeyByID should only be called once per key ID
	enc1, err := c.Encrypt(ctx, nil, []byte("test"))
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		_, err := c.Decrypt(ctx, nil, enc1)
		require.NoError(t, err)
	}
	// Key 1 was already cached during encrypt, so GetKeyByID should be 0
	assert.Equal(t, int64(0), ks.getByIDHits.Load(), "GetKeyByID should not be called when key is already cached from encrypt")
}

func TestCache_ConcurrentEncryptDecrypt(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("concurrent-kek")
	ks := newMemKeyStore()
	seedActiveKey(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Pre-encrypt some data to decrypt concurrently
	var encrypted [][]byte
	for i := 0; i < 10; i++ {
		enc, err := c.Encrypt(ctx, nil, []byte(fmt.Sprintf("data-%d", i)))
		require.NoError(t, err)
		encrypted = append(encrypted, enc)
	}

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent encrypt
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := c.Encrypt(ctx, nil, []byte(fmt.Sprintf("concurrent-%d", idx)))
			assert.NoError(t, err)
		}(i)
	}

	// Concurrent decrypt
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			enc := encrypted[idx%len(encrypted)]
			_, err := c.Decrypt(ctx, nil, enc)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

func TestCache_ClearCacheMidFlight(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("clear-kek")
	ks := newMemKeyStore()
	seedActiveKey(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	var wg sync.WaitGroup
	const goroutines = 20

	// Goroutines encrypting while cache is being cleared
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				enc, err := c.Encrypt(ctx, nil, []byte(fmt.Sprintf("data-%d-%d", idx, j)))
				if err != nil {
					continue // may fail transiently during clear, that's ok
				}
				// Should always round-trip
				dec, err := c.Decrypt(ctx, nil, enc)
				if err == nil {
					assert.Equal(t, fmt.Sprintf("data-%d-%d", idx, j), string(dec))
				}
			}
		}(i)
	}

	// Periodically clear cache
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.ClearCache()
		}()
	}

	wg.Wait()
}

func TestCache_ClearCacheRefetches(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("refetch-kek")
	ks := newCountingKeyStore()
	seedActiveKeyOnCounting(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// First encrypt populates cache
	_, err := c.Encrypt(ctx, nil, []byte("before-clear"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), ks.activeKeyHits.Load())

	// Clear and encrypt again — should re-fetch
	c.ClearCache()
	_, err = c.Encrypt(ctx, nil, []byte("after-clear"))
	require.NoError(t, err)
	assert.Equal(t, int64(2), ks.activeKeyHits.Load())
}

func TestCache_EncryptFailsOnTransientKeyStoreError(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("fail-kek")
	ks := &failingKeyStore{
		memKeyStore: *newMemKeyStore(),
		failActive:  true,
	}
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	_, err := c.Encrypt(ctx, nil, []byte("should-fail"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get active key")
}

func TestCache_DecryptKeyNotFound(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("notfound-kek")
	ks := newMemKeyStore()
	seedActiveKey(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Craft a payload referencing a non-existent key ID
	fakePayload := "999/" + base64.URLEncoding.EncodeToString([]byte("garbage-ciphertext-that-is-long-enough-for-nonce-and-data"))
	_, err := c.Decrypt(ctx, nil, []byte(fakePayload))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get key 999")
}

func TestCache_DecryptCorruptedWrappedDEK(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("corrupt-kek")
	ks := newMemKeyStore()
	ctx := context.Background()

	// Create a key with invalid wrapped DEK (valid base64 but wrong encryption)
	wrongKEK := newTestLocalKEK("wrong-kek")
	dek := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, dek)
	require.NoError(t, err)
	wrongWrapped, err := wrongKEK.WrapKey(ctx, dek)
	require.NoError(t, err)
	_, err = ks.CreateKey(ctx, base64.URLEncoding.EncodeToString(wrongWrapped), EncryptionKeyActive)
	require.NoError(t, err)

	c := NewEnvelopeCipher(kek, ks) // uses different KEK than what wrapped the DEK
	_, err = c.Encrypt(ctx, nil, []byte("should-fail"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unwrap key")
}

func TestCache_EncryptAfterRetiredKeyStillWorksCached(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("retire-kek")
	ks := newMemKeyStore()
	seedActiveKey(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Warm the cache
	enc1, err := c.Encrypt(ctx, nil, []byte("before-retire"))
	require.NoError(t, err)

	// Retire the key in the store (but cache still has it)
	key1, _ := ks.GetKeyByID(ctx, 1)
	key1.Status = EncryptionKeyRetired
	ks.UpdateKey(ctx, key1)

	// Should still encrypt using cached DEK
	enc2, err := c.Encrypt(ctx, nil, []byte("after-retire"))
	require.NoError(t, err)

	// Both should decrypt
	dec1, err := c.Decrypt(ctx, nil, enc1)
	require.NoError(t, err)
	assert.Equal(t, "before-retire", string(dec1))

	dec2, err := c.Decrypt(ctx, nil, enc2)
	require.NoError(t, err)
	assert.Equal(t, "after-retire", string(dec2))
}

func TestCache_InvalidBase64WrappedKey(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("b64-kek")
	ks := newMemKeyStore()
	ctx := context.Background()

	// Create key with invalid base64
	_, err := ks.CreateKey(ctx, "not-valid-base64!!!", EncryptionKeyActive)
	require.NoError(t, err)

	c := NewEnvelopeCipher(kek, ks)
	_, err = c.Encrypt(ctx, nil, []byte("should-fail"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode wrapped key")
}

func TestCache_MultipleKeyIDsCachedSimultaneously(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("multi-kek")
	ks := newCountingKeyStore()
	ctx := context.Background()

	// Create two keys
	seedActiveKeyOnCounting(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)

	// Encrypt with key 1
	enc1, err := c.Encrypt(ctx, nil, []byte("key-1-data"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(enc1), "1/"))

	// Retire key 1, create key 2
	key1, _ := ks.inner.GetKeyByID(ctx, 1)
	key1.Status = EncryptionKeyRetired
	ks.inner.UpdateKey(ctx, key1)
	seedActiveKeyOnCounting(t, ks, kek)
	c.ClearCache()

	// Encrypt with key 2
	enc2, err := c.Encrypt(ctx, nil, []byte("key-2-data"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(enc2), "2/"))

	// Reset counters to measure decrypt cache behavior
	ks.getByIDHits.Store(0)

	// Alternate decrypts between key 1 and key 2
	for i := 0; i < 20; i++ {
		dec1, err := c.Decrypt(ctx, nil, enc1)
		require.NoError(t, err)
		assert.Equal(t, "key-1-data", string(dec1))

		dec2, err := c.Decrypt(ctx, nil, enc2)
		require.NoError(t, err)
		assert.Equal(t, "key-2-data", string(dec2))
	}

	// Key 2 was cached during encrypt. Key 1 cache was cleared, so one fetch for key 1.
	assert.Equal(t, int64(1), ks.getByIDHits.Load(), "should only fetch key 1 once, key 2 from encrypt cache")
}

func TestCache_EmptyPlaintextRoundTrip(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("empty-kek")
	ks := newMemKeyStore()
	seedActiveKey(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Zero-length byte slice (not nil)
	enc, err := c.Encrypt(ctx, nil, []byte{})
	require.NoError(t, err)

	dec, err := c.Decrypt(ctx, nil, enc)
	require.NoError(t, err)
	assert.Empty(t, dec, "decrypted empty plaintext should be empty")
}

func TestCache_LargePlaintextRoundTrip(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("large-kek")
	ks := newMemKeyStore()
	seedActiveKey(t, ks, kek)
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// 1MB+ plaintext
	large := make([]byte, 1024*1024+37)
	_, err := io.ReadFull(rand.Reader, large)
	require.NoError(t, err)

	enc, err := c.Encrypt(ctx, nil, large)
	require.NoError(t, err)

	dec, err := c.Decrypt(ctx, nil, enc)
	require.NoError(t, err)
	assert.Equal(t, large, dec)
}
