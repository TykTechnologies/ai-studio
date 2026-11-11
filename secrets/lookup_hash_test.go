package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
)

func TestLookupHash(t *testing.T) {
	const value = "an-app-gateway-key"

	t.Setenv("TYK_AI_SECRET_KEY", "lookup-hash-test-key")
	keyed := LookupHash(value)
	if !strings.HasPrefix(keyed, LookupHashKeyedPrefix) || LookupHashPrefix() != LookupHashKeyedPrefix {
		t.Fatalf("with a key: %q, prefix %q", keyed, LookupHashPrefix())
	}
	if !credhash.IsHash(strings.TrimPrefix(keyed, LookupHashKeyedPrefix)) {
		t.Errorf("%q is not a credhash hash after its prefix", keyed)
	}
	if keyed != LookupHash(value) {
		t.Error("the hash of one value differs between calls")
	}
	if keyed == LookupHash(value+"x") {
		t.Error("two values share a hash")
	}
	if strings.Contains(keyed, value) || len(keyed) > LookupHashMaxLen {
		t.Errorf("unexpected form: %q", keyed)
	}
	plain := sha256.Sum256([]byte(value))
	if strings.Contains(keyed, hex.EncodeToString(plain[:])) {
		t.Error("the keyed hash is the plain SHA-256")
	}

	// Another key, another hash: with the wrong key nothing is found.
	t.Setenv("TYK_AI_SECRET_KEY", "another-lookup-test-key")
	if LookupHash(value) == keyed {
		t.Error("the hash does not depend on the key")
	}

	// No key, or one too short for the keyed construction: unkeyed.
	for _, key := range []string{"", "short"} {
		t.Setenv("TYK_AI_SECRET_KEY", key)
		got := LookupHash(value)
		if got != LookupHashPlainPrefix+hex.EncodeToString(plain[:]) || LookupHashPrefix() != LookupHashPlainPrefix {
			t.Errorf("key %q: %q, prefix %q", key, got, LookupHashPrefix())
		}
	}
}
