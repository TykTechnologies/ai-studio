package secrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers for cache tests ---

// countingKeyStore wraps a memKeyStore and counts calls to GetKeyByID and CreateKey.
type countingKeyStore struct {
	inner         *memKeyStore
	getByIDHits   atomic.Int64
	createKeyHits atomic.Int64
}

func newCountingKeyStore() *countingKeyStore {
	return &countingKeyStore{inner: newMemKeyStore()}
}

func (c *countingKeyStore) GetKeyByID(ctx context.Context, id uint) (*EncryptionKey, error) {
	c.getByIDHits.Add(1)
	return c.inner.GetKeyByID(ctx, id)
}

func (c *countingKeyStore) CreateKey(ctx context.Context, wrappedKey string, status string, objectType string, objectID uint) (*EncryptionKey, error) {
	c.createKeyHits.Add(1)
	return c.inner.CreateKey(ctx, wrappedKey, status, objectType, objectID)
}

func (c *countingKeyStore) ListKeys(ctx context.Context) ([]EncryptionKey, error) {
	return c.inner.ListKeys(ctx)
}

func (c *countingKeyStore) UpdateKey(ctx context.Context, key *EncryptionKey) error {
	return c.inner.UpdateKey(ctx, key)
}

// failingKeyStore returns errors from GetKeyByID and CreateKey.
type failingKeyStore struct {
	memKeyStore
	failGetByID bool
	failCreate  bool
}

func (f *failingKeyStore) GetKeyByID(_ context.Context, id uint) (*EncryptionKey, error) {
	if f.failGetByID {
		return nil, fmt.Errorf("key not found")
	}
	return f.memKeyStore.GetKeyByID(context.Background(), id)
}

func (f *failingKeyStore) CreateKey(_ context.Context, wrappedKey string, status string, objectType string, objectID uint) (*EncryptionKey, error) {
	if f.failCreate {
		return nil, fmt.Errorf("db write error")
	}
	return f.memKeyStore.CreateKey(context.Background(), wrappedKey, status, objectType, objectID)
}

// --- 1. Per-object DEK tests ---

func TestCache_EachEncryptCreatesNewKey(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("cache-kek")
	ks := newCountingKeyStore()
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Encrypt 10 times — should create 10 keys (per-object DEK)
	for i := 0; i < 10; i++ {
		_, err := c.Encrypt(ctx, nil, []byte(fmt.Sprintf("data-%d", i)))
		require.NoError(t, err)
	}
	assert.Equal(t, int64(10), ks.createKeyHits.Load(), "each encrypt should create a new key")
}

func TestCache_DecryptUsesCache(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("cache-kek")
	ks := newCountingKeyStore()
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Encrypt caches the DEK for future decrypts
	enc, err := c.Encrypt(ctx, nil, []byte("test"))
	require.NoError(t, err)

	// Decrypt 50 times — should use cache, no GetKeyByID calls
	for i := 0; i < 50; i++ {
		_, err := c.Decrypt(ctx, nil, enc)
		require.NoError(t, err)
	}
	assert.Equal(t, int64(0), ks.getByIDHits.Load(), "decrypt should use cache from encrypt")
}

func TestCache_DecryptCacheMissCallsGetKeyByID(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("miss-kek")
	ks := newCountingKeyStore()
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	// Encrypt to create a key
	enc, err := c.Encrypt(ctx, nil, []byte("data"))
	require.NoError(t, err)

	// Clear cache so decrypt must fetch from store
	c.ClearCache()

	_, err = c.Decrypt(ctx, nil, enc)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ks.getByIDHits.Load(), "cache miss should trigger GetKeyByID")

	// Second decrypt should use cache
	_, err = c.Decrypt(ctx, nil, enc)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ks.getByIDHits.Load(), "second decrypt should use cache")
}

func TestCache_ConcurrentEncryptDecrypt(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("concurrent-kek")
	ks := newMemKeyStore()
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
					continue
				}
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

func TestCache_EncryptFailsOnCreateKeyError(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("fail-kek")
	ks := &failingKeyStore{
		memKeyStore: *newMemKeyStore(),
		failCreate:  true,
	}
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

	_, err := c.Encrypt(ctx, nil, []byte("should-fail"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store dek")
}

func TestCache_DecryptKeyNotFound(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("notfound-kek")
	ks := newMemKeyStore()
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

	// Create a key wrapped with a different KEK (valid base64 but wrong encryption)
	wrongKEK := newTestLocalKEK("wrong-kek")
	dek := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, dek)
	require.NoError(t, err)
	wrongWrapped, err := wrongKEK.WrapKey(ctx, dek)
	require.NoError(t, err)
	key, err := ks.CreateKey(ctx, base64.URLEncoding.EncodeToString(wrongWrapped), EncryptionKeyActive, "", 0)
	require.NoError(t, err)

	c := NewEnvelopeCipher(kek, ks) // uses different KEK than what wrapped the DEK

	// Decrypt should fail when trying to unwrap the DEK with the wrong KEK
	fakePayload := fmt.Sprintf("%d/%s", key.ID, base64.URLEncoding.EncodeToString([]byte("some-ciphertext-long-enough-for-nonce")))
	_, err = c.Decrypt(ctx, nil, []byte(fakePayload))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unwrap key")
}

func TestCache_InvalidBase64WrappedKey(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("b64-kek")
	ks := newMemKeyStore()
	ctx := context.Background()

	// Create key with invalid base64
	key, err := ks.CreateKey(ctx, "not-valid-base64!!!", EncryptionKeyActive, "", 0)
	require.NoError(t, err)

	c := NewEnvelopeCipher(kek, ks)

	// Decrypt with this key should fail on base64 decode
	fakePayload := fmt.Sprintf("%d/%s", key.ID, base64.URLEncoding.EncodeToString([]byte("some-ciphertext-long-enough-for-nonce")))
	_, err = c.Decrypt(ctx, nil, []byte(fakePayload))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode wrapped key")
}

func TestCache_MultipleKeyIDsCachedSimultaneously(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("multi-kek")
	ks := newCountingKeyStore()
	ctx := context.Background()

	c := NewEnvelopeCipher(kek, ks)

	// Each encrypt creates its own key
	enc1, err := c.Encrypt(ctx, nil, []byte("key-1-data"))
	require.NoError(t, err)

	enc2, err := c.Encrypt(ctx, nil, []byte("key-2-data"))
	require.NoError(t, err)

	// Both should use different key IDs
	assert.NotEqual(t, string(enc1)[:2], string(enc2)[:2], "should use different key IDs")

	// Both cached from encrypt — no GetKeyByID calls needed
	ks.getByIDHits.Store(0)

	for i := 0; i < 20; i++ {
		dec1, err := c.Decrypt(ctx, nil, enc1)
		require.NoError(t, err)
		assert.Equal(t, "key-1-data", string(dec1))

		dec2, err := c.Decrypt(ctx, nil, enc2)
		require.NoError(t, err)
		assert.Equal(t, "key-2-data", string(dec2))
	}

	assert.Equal(t, int64(0), ks.getByIDHits.Load(), "all keys should be cached from encrypt")
}

func TestCache_EmptyPlaintextRoundTrip(t *testing.T) {
	t.Parallel()
	kek := newTestLocalKEK("empty-kek")
	ks := newMemKeyStore()
	c := NewEnvelopeCipher(kek, ks)
	ctx := context.Background()

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
