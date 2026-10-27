package nativeapi

import (
	"crypto/fips140"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func llmWithKeys(cred, secret, prev string, prevExpires *time.Time) *models.LLM {
	return &models.LLM{
		APIKey:                       cred,
		ResourceSigningSecret:        secret,
		ResourceSigningSecretPrev:    prev,
		ResourceSigningPrevExpiresAt: prevExpires,
	}
}

func mustLLMCodec(t *testing.T, llm *models.LLM, now time.Time) *Codec {
	t.Helper()
	c, err := CodecForLLM(llm, now)
	if err != nil {
		t.Fatalf("CodecForLLM: %v", err)
	}
	return c
}

func TestSecretKeyIsDomainSeparatedFromCredentialKey(t *testing.T) {
	s, err := SecretKey("same-material")
	if err != nil {
		t.Fatal(err)
	}
	c, err := CredentialKey("same-material")
	if err != nil {
		t.Fatal(err)
	}
	if s.KID() == c.KID() {
		t.Fatal("a secret and a credential with the same bytes must not derive the same key")
	}
	if _, err := SecretKey(""); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("SecretKey(\"\") = %v, want ErrEmptyKey", err)
	}
}

func TestCodecForLLMWithoutSecretUsesCredential(t *testing.T) {
	now := time.Now()
	c := mustLLMCodec(t, llmWithKeys("sk-cred", "", "", nil), now)
	if c.KID() != mustCodec(t, "sk-cred").KID() {
		t.Fatal("an LLM without a signing secret must sign with the vendor-credential key")
	}
	if _, err := CodecForLLM(llmWithKeys("", "", "", nil), now); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("no secret and no credential = %v, want ErrEmptyKey", err)
	}
}

func TestCodecForLLMSecretSurvivesCredentialRotation(t *testing.T) {
	now := time.Now()
	scope := Scope{LLMID: 3, AppID: 9}
	before := mustLLMCodec(t, llmWithKeys("old-cred", "secret-1", "", nil), now)
	w, err := before.Wrap(testBatch, "msgbatch_1", scope)
	if err != nil {
		t.Fatal(err)
	}
	after := mustLLMCodec(t, llmWithKeys("new-cred", "secret-1", "", nil), now)
	if got, err := after.Unwrap(testBatch, w, scope); err != nil || got != "msgbatch_1" {
		t.Fatalf("Unwrap after a credential rotation = %q, %v", got, err)
	}
	// A secret needs no credential at all.
	if _, err := CodecForLLM(llmWithKeys("", "secret-1", "", nil), now); err != nil {
		t.Fatalf("CodecForLLM with a secret and no credential: %v", err)
	}
}

func TestCodecForLLMPreviousKeyGrace(t *testing.T) {
	now := time.Now()
	later, earlier := now.Add(time.Hour), now.Add(-time.Hour)
	scope := Scope{LLMID: 3, AppID: 9}
	wrapWith := func(llm *models.LLM) string {
		t.Helper()
		w, err := mustLLMCodec(t, llm, now).Wrap(testBatch, "msgbatch_1", scope)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	oldSecretID := wrapWith(llmWithKeys("cred", "secret-old", "", nil))
	credentialID := wrapWith(llmWithKeys("cred", "", "", nil))

	cases := []struct {
		name string
		llm  *models.LLM
		id   string
		want error
	}{
		{"previous secret in grace", llmWithKeys("cred", "secret-new", "secret-old", &later), oldSecretID, nil},
		{"previous secret expired", llmWithKeys("cred", "secret-new", "secret-old", &earlier), oldSecretID, ErrKeyMismatch},
		{"previous secret without expiry", llmWithKeys("cred", "secret-new", "secret-old", nil), oldSecretID, ErrKeyMismatch},
		{"legacy credential in grace", llmWithKeys("cred", "secret-new", models.ResourceSigningLegacyKey, &later), credentialID, nil},
		{"legacy credential expired", llmWithKeys("cred", "secret-new", models.ResourceSigningLegacyKey, &earlier), credentialID, ErrKeyMismatch},
		{"legacy credential after a credential rotation", llmWithKeys("cred-2", "secret-new", models.ResourceSigningLegacyKey, &later), credentialID, ErrKeyMismatch},
		{"credential ID on an LLM with a secret and no legacy grace", llmWithKeys("cred", "secret-new", "", nil), credentialID, ErrKeyMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := mustLLMCodec(t, tc.llm, now)
			got, err := c.Unwrap(testBatch, tc.id, scope)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Unwrap = %q, %v; want %v", got, err, tc.want)
			}
			if tc.want == nil && got != "msgbatch_1" {
				t.Fatalf("Unwrap = %q", got)
			}
			// New IDs are always signed with the current secret.
			if fresh := wrapWith(tc.llm); fresh == tc.id {
				t.Fatal("a codec must not mint IDs with its previous key")
			}
		})
	}
}

func TestKeyringCodecChecksScopeWithPreviousKey(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	old := mustLLMCodec(t, llmWithKeys("cred", "secret-old", "", nil), now)
	w, err := old.Wrap(testBatch, "msgbatch_1", Scope{LLMID: 3, AppID: 9})
	if err != nil {
		t.Fatal(err)
	}
	c := mustLLMCodec(t, llmWithKeys("cred", "secret-new", "secret-old", &later), now)
	if _, err := c.Unwrap(testBatch, w, Scope{LLMID: 3, AppID: 10}); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("another App's ID under the previous key = %v, want ErrBadSignature", err)
	}
}

func storedDerivedKey(t *testing.T, cred string) string {
	t.Helper()
	raw, err := models.DeriveCredentialSigningKey(cred)
	if err != nil {
		t.Fatal(err)
	}
	return models.ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

// The vendor-credential key keeps its derivation (IDs minted before 2.3
// carry its kid), now computed in models for the backfill.
func TestCredentialKeyDerivationUnchanged(t *testing.T) {
	if fips140.Enabled() {
		t.Skip("pins the standard-build derivation; FIPS mode hashes a short credential first (models.HKDFKeyMaterial)")
	}
	raw, err := hkdf.Key(sha256.New, []byte("sk-cred"), nil, "tyk-native-resource/v1", sha256.Size)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	k, err := CredentialKey("sk-cred")
	if err != nil {
		t.Fatal(err)
	}
	if k.KID() != hex.EncodeToString(sum[:4]) {
		t.Fatalf("credential key kid changed: %s", k.KID())
	}
}

// The backfill stores the derived vendor-credential key, so IDs it signed
// still verify after the vendor credential changes, until the grace ends.
func TestCodecForLLMStoredCredentialKeySurvivesCredentialChange(t *testing.T) {
	now := time.Now()
	later, earlier := now.Add(time.Hour), now.Add(-time.Hour)
	scope := Scope{LLMID: 3, AppID: 9}
	old, err := NewCodec("cred-at-upgrade")
	if err != nil {
		t.Fatal(err)
	}
	id, err := old.Wrap(testBatch, "msgbatch_1", scope)
	if err != nil {
		t.Fatal(err)
	}
	stored := storedDerivedKey(t, "cred-at-upgrade")

	for name, tc := range map[string]struct {
		llm  *models.LLM
		want error
	}{
		"same credential":    {llmWithKeys("cred-at-upgrade", "secret", stored, &later), nil},
		"credential changed": {llmWithKeys("new-cred", "secret", stored, &later), nil},
		"no credential now":  {llmWithKeys("", "secret", stored, &later), nil},
		"grace over":         {llmWithKeys("new-cred", "secret", stored, &earlier), ErrKeyMismatch},
		"corrupt stored key": {llmWithKeys("cred-at-upgrade", "secret", models.ResourceSigningDerivedKeyPrefix+"!!", &later), ErrKeyMismatch},
		"short stored key":   {llmWithKeys("cred-at-upgrade", "secret", models.ResourceSigningDerivedKeyPrefix+"AAAA", &later), ErrKeyMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := mustLLMCodec(t, tc.llm, now).Unwrap(testBatch, id, scope)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Unwrap = %q, %v; want %v", got, err, tc.want)
			}
		})
	}

	k, ok, err := PreviousKeyForLLM(llmWithKeys("new-cred", "secret", stored, &earlier))
	if err != nil || !ok || k.KID() != old.KID() {
		t.Fatalf("PreviousKeyForLLM = %v, %v, %v; want the upgrade-time kid %s", k.KID(), ok, err, old.KID())
	}
	if _, ok, _ := PreviousKeyForLLM(llmWithKeys("c", "secret", "", nil)); ok {
		t.Fatal("no previous key")
	}
}
