package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestDataKeySealAndOpen(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "data-key-test-secret")
	salt := bytes.Repeat([]byte{7}, DataKeySaltLength)

	before := KeyDerivations()
	var sealed []string
	for i := 0; i < 200; i++ {
		s, err := SealWithDataKey(salt, "an-app-key", "credential\x00key-1")
		if err != nil {
			t.Fatal(err)
		}
		sealed = append(sealed, s)
	}
	for _, s := range sealed {
		plain, err := OpenWithDataKey(salt, s, "credential\x00key-1")
		if err != nil || plain != "an-app-key" {
			t.Fatalf("open: %q, %v", plain, err)
		}
	}
	if got := KeyDerivations() - before; got != 1 {
		t.Errorf("400 seals and opens derived %d keys, want 1", got)
	}
	if sealed[0] == sealed[1] {
		t.Error("two seals of one value are identical: the nonce is not random")
	}
	if !strings.HasPrefix(sealed[0], DataKeyCurrentPrefix()) || !IsDataKeyValue(sealed[0]) || strings.HasPrefix(sealed[0], "$ENC/") {
		t.Errorf("unexpected form %q", sealed[0])
	}
	if strings.Contains(sealed[0], "an-app-key") {
		t.Error("the plaintext is in the stored value")
	}

	// Bound to the row, the salt and the key.
	if _, err := OpenWithDataKey(salt, sealed[0], "credential\x00key-2"); err == nil {
		t.Error("a ciphertext opened as another row's")
	}
	if _, err := OpenWithDataKey(bytes.Repeat([]byte{8}, DataKeySaltLength), sealed[0], "credential\x00key-1"); err == nil {
		t.Error("opened with another database's salt")
	}
	tampered := sealed[0][:len(sealed[0])-2] + "AA"
	if _, err := OpenWithDataKey(salt, tampered, "credential\x00key-1"); err == nil {
		t.Error("a tampered ciphertext opened")
	}
	t.Setenv("TYK_AI_SECRET_KEY", "another-secret")
	if _, err := OpenWithDataKey(salt, sealed[0], "credential\x00key-1"); err == nil {
		t.Error("opened with another TYK_AI_SECRET_KEY")
	}

	// No key or no salt: an error, never plaintext passed off as sealed.
	if _, err := SealWithDataKey(nil, "x", "a"); err != ErrDataKeyUnavailable {
		t.Errorf("no salt: %v", err)
	}
	t.Setenv("TYK_AI_SECRET_KEY", "")
	if _, err := SealWithDataKey(salt, "x", "a"); err != ErrDataKeyUnavailable {
		t.Errorf("no key: %v", err)
	}
}

// A FIPS-mode process writes the PBKDF2 form and reads the scrypt form a
// standard build wrote, and the other way round.
func TestDataKeyFormsAcrossFIPSAndStandardBuilds(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "data-key-test-secret")
	salt := bytes.Repeat([]byte{9}, DataKeySaltLength)
	prev := fipsEnabled
	t.Cleanup(func() { fipsEnabled = prev })

	fipsEnabled = func() bool { return false }
	standard, err := SealWithDataKey(salt, "k", "row")
	if err != nil || !strings.HasPrefix(standard, DataKeyPrefix+"s:") {
		t.Fatalf("standard form: %q, %v", standard, err)
	}
	fipsEnabled = func() bool { return true }
	fips, err := SealWithDataKey(salt, "k", "row")
	if err != nil || !strings.HasPrefix(fips, DataKeyPrefix+"p") {
		t.Fatalf("FIPS form: %q, %v", fips, err)
	}
	if strings.HasPrefix(standard, DataKeyCurrentPrefix()) {
		t.Error("a FIPS process takes the scrypt form for its current one")
	}
	for _, v := range []string{standard, fips} {
		if plain, err := OpenWithDataKey(salt, v, "row"); err != nil || plain != "k" {
			t.Errorf("FIPS process reading %q: %q, %v", v[:8], plain, err)
		}
	}
	fipsEnabled = func() bool { return false }
	if plain, err := OpenWithDataKey(salt, fips, "row"); err != nil || plain != "k" {
		t.Errorf("standard process reading the FIPS form: %q, %v", plain, err)
	}
	for _, bad := range []string{DataKeyPrefix + "x:AAAA", DataKeyPrefix + "p0:AAAA", DataKeyPrefix + "pzz:AAAA", DataKeyPrefix + "noheader", "$ENC/v2:AAAA"} {
		if _, err := OpenWithDataKey(salt, bad, "row"); err == nil {
			t.Errorf("%q opened", bad)
		}
	}
}
