package credhash

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/fipstest"
)

// Vectors are fixed: a change to the construction that alters any of them
// invalidates every stored hash and needs a new label and Prefix instead.
// The second one was checked independently with OpenSSL:
//
//	key=$(openssl kdf -keylen 32 -kdfopt digest:SHA2-256 \
//	  -kdfopt key:0123456789abcdef0123456789abcdef \
//	  -kdfopt info:credential-hash-v1 HKDF | tr -d ':')
//	printf 'tyk-test-credential' | openssl dgst -sha256 -mac HMAC -macopt hexkey:$key
//
// The microgateway module asserts the same vectors
// (microgateway/internal/services/credential_hash_test.go), so the hub and
// the edge are known to agree.
var vectors = []struct {
	key, credential, hash string
}{
	{"0123456789abcdef0123456789abcdef", "", "v1:c18a2b2c4a39d503bbbcce02068c0b8cddb68190c489dadb4b52da410e72dbf5"},
	{"0123456789abcdef0123456789abcdef", "tyk-test-credential", "v1:03c819a3f64b42c74f216528f1141cd52aedf44c251b106b18c42a32b904ce45"},
	{"0123456789abcdef0123456789abcdef", "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f", "v1:f1b41918681329798b132b8035ad7607044fd01d970ae48fd5ce08a5c5cb28eb"},
	{"fedcba9876543210fedcba9876543210", "", "v1:6b0ce5a5529404224faac591e1dd0c9aa9e41f54c4a3990aff7aa507f21ed886"},
	{"fedcba9876543210fedcba9876543210", "tyk-test-credential", "v1:ad48c55e0b01ee0b632d24a6fd6c7007bd915be963fef33ecba2c60a0b1cf1e8"},
	{"fedcba9876543210fedcba9876543210", "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f", "v1:d1fe04b9139c59044b11739bed36670c29f5d3d629a59ed63a2db0b7548b2135"},
}

func mustNew(t testing.TB, key string) *Hasher {
	t.Helper()
	h, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func TestHashVectors(t *testing.T) {
	for _, v := range vectors {
		got := mustNew(t, v.key).Hash(v.credential)
		if got != v.hash {
			t.Errorf("Hash(key %q, credential %q) = %s, want %s", v.key, v.credential, got, v.hash)
		}
	}
}

func TestHashForm(t *testing.T) {
	h := mustNew(t, vectors[0].key)
	for _, cred := range []string{"", "tyk-test-credential", strings.Repeat("x", 4096)} {
		got := h.Hash(cred)
		if len(got) != Len || Len != 67 {
			t.Errorf("len(Hash) = %d, Len = %d, want 67", len(got), Len)
		}
		if !strings.HasPrefix(got, Prefix) {
			t.Errorf("Hash = %q, want prefix %q", got, Prefix)
		}
		if !IsHash(got) {
			t.Errorf("IsHash(%q) = false", got)
		}
		if cred != "" && strings.Contains(got, cred) {
			t.Errorf("hash %q contains the credential", got)
		}
	}
	if h.Hash("same") != h.Hash("same") {
		t.Error("Hash is not deterministic")
	}
	if h.Hash("one") == h.Hash("two") {
		t.Error("different credentials gave the same hash")
	}
}

func TestDifferentKeysGiveDifferentHashes(t *testing.T) {
	a := mustNew(t, "0123456789abcdef0123456789abcdef")
	b := mustNew(t, "0123456789abcdef0123456789abcdeg")
	if a.Hash("tyk-test-credential") == b.Hash("tyk-test-credential") {
		t.Fatal("two encryption keys gave the same hash")
	}
}

// The label is part of the derivation: the same encryption key under another
// label (here the snapshot checksum's, grpc/snapshot_secret_digests.go), or
// used as the HMAC key directly, gives another value.
func TestLabelIsPartOfTheDerivation(t *testing.T) {
	const key, cred = "0123456789abcdef0123456789abcdef", "tyk-test-credential"
	want := mustNew(t, key).Hash(cred)

	derive := func(info string) string {
		k, err := hkdf.Key(sha256.New, []byte(key), nil, info, sha256.Size)
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, k)
		mac.Write([]byte(cred))
		return Prefix + hex.EncodeToString(mac.Sum(nil))
	}
	if got := derive(label); got != want {
		t.Fatalf("derivation with the label = %s, want %s", got, want)
	}
	for _, other := range []string{"midsommar:microgateway:snapshot-checksum:v1", "credential-hash-v2", ""} {
		if derive(other) == want {
			t.Errorf("label %q gave the credential hash", other)
		}
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(cred))
	if Prefix+hex.EncodeToString(mac.Sum(nil)) == want {
		t.Error("the encryption key is used as the HMAC key without derivation")
	}
}

func TestNewRejectsShortKeys(t *testing.T) {
	for _, key := range []string{"", "short", "thirteen-char"} {
		if _, err := New(key); !errors.Is(err, ErrKeyTooShort) {
			t.Errorf("New(%q) error = %v, want ErrKeyTooShort", key, err)
		}
	}
	if _, err := New("fourteen-chars"); err != nil {
		t.Errorf("New with a 14-byte key: %v", err)
	}
}

func TestIsHash(t *testing.T) {
	good := vectors[0].hash
	for s, want := range map[string]bool{
		good:                             true,
		"":                               false,
		good[:len(good)-1]:               false,
		good + "0":                       false,
		"v2:" + good[len(Prefix):]:       false,
		strings.ToUpper(good):            false,
		Prefix + strings.Repeat("g", 64): false,
		"5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f": false,
	} {
		if got := IsHash(s); got != want {
			t.Errorf("IsHash(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestHasherIsSafeForConcurrentUse(t *testing.T) {
	h := mustNew(t, vectors[0].key)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if got := h.Hash(vectors[1].credential); got != vectors[1].hash {
					t.Errorf("concurrent Hash = %s", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestFIPSStrict is this package's part of the FIPS canary coverage: under
// make test-fips (GOFIPS140=v1.0.0, GODEBUG=fips140=only) it first proves
// strict mode is enforced, then that the whole construction runs in it, where
// a non-approved primitive or a short key would fail. Skipped otherwise;
// TestHashVectors covers the values in every mode.
func TestFIPSStrict(t *testing.T) {
	fipstest.Canary(t)
	for _, v := range vectors {
		if got := mustNew(t, v.key).Hash(v.credential); got != v.hash {
			t.Errorf("under fips140=only: Hash = %s, want %s", got, v.hash)
		}
	}
	// The shortest key New accepts is one strict mode accepts too.
	mustNew(t, "fourteen-chars").Hash("x")
}

func BenchmarkHash(b *testing.B) {
	h := mustNew(b, vectors[0].key)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Hash(vectors[2].credential)
	}
}
