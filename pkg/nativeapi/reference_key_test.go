package nativeapi

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// An App that can read an LLM sees its credential reference ("$SECRET/NAME").
// No signing key may ever be the one derived from that text, or any such App
// could sign an ID over another App's resource (midsommar-clean-6bg5.94).

// forgerCodec signs as someone who knows only the reference text would.
func forgerCodec(t *testing.T, reference string) *Codec {
	t.Helper()
	raw, err := hkdf.Key(sha256.New, []byte(reference), nil, "tyk-native-resource/v1", sha256.Size)
	if err != nil {
		t.Fatal(err)
	}
	return NewKeyringCodec(keyOf(raw))
}

func TestNoKeyIsDerivedFromAReferenceText(t *testing.T) {
	for _, ref := range []string{"$SECRET/OPENAI_KEY", "$ENV/OPENAI_API_KEY", "$ENV/"} {
		if _, err := CredentialKey(ref); !errors.Is(err, models.ErrUnresolvedCredential) {
			t.Fatalf("CredentialKey(%q) = %v, want ErrUnresolvedCredential", ref, err)
		}
		if _, err := NewCodec(ref); err == nil {
			t.Fatalf("NewCodec(%q) built a codec from a reference", ref)
		}
		// An LLM without a signing secret whose credential did not resolve
		// has no codec: it neither mints nor accepts IDs.
		if _, err := CodecForLLM(llmWithKeys(ref, "", "", nil), time.Now()); err == nil {
			t.Fatalf("CodecForLLM with credential %q built a codec", ref)
		}
	}
}

func TestForgedIDFromTheReferenceNameIsRefused(t *testing.T) {
	const reference = "$SECRET/OPENAI_KEY"
	now := time.Now()
	exp := now.Add(time.Hour)
	rt := testBatch
	victim := Scope{LLMID: 7, AppID: 1}
	forged, err := forgerCodec(t, reference).Wrap(rt, "msgbatch_01app1", victim)
	if err != nil {
		t.Fatal(err)
	}

	// The legacy sentinel stands for "the key of the LLM's current
	// credential". With an unresolved reference there is no such key.
	legacy := mustLLMCodec(t, llmWithKeys(reference, "hub-secret", models.ResourceSigningLegacyKey, &exp), now)
	if _, err := legacy.Unwrap(rt, forged, victim); err == nil {
		t.Fatalf("legacy sentinel with an unresolved credential accepted a forged id: %v", err)
	}

	// The state a pre-fix 2.3 build left in the database: the stored
	// "vendor-credential" key is the forger's key. This is what
	// models.ClearReferenceDerivedSigningKeys removes on start; until then
	// the forged id verifies, which is the reported hole.
	raw, _ := hkdf.Key(sha256.New, []byte(reference), nil, "tyk-native-resource/v1", sha256.Size)
	poisoned := models.ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	before := mustLLMCodec(t, llmWithKeys(reference, "hub-secret", poisoned, &exp), now)
	if _, err := before.Unwrap(rt, forged, victim); err != nil {
		t.Fatalf("the poisoned state should reproduce the finding, got %v", err)
	}
	after := mustLLMCodec(t, llmWithKeys(reference, "hub-secret", "", nil), now)
	if _, err := after.Unwrap(rt, forged, victim); err == nil {
		t.Fatalf("after the previous key is cleared the forged id must be refused: %v", err)
	}

	// IDs handed out before the upgrade were signed with the resolved
	// credential and stay valid through the grace period.
	real, err := models.DeriveCredentialSigningKey("sk-the-real-key")
	if err != nil {
		t.Fatal(err)
	}
	genuine, err := NewKeyringCodec(keyOf(real)).Wrap(rt, "msgbatch_01app1", victim)
	if err != nil {
		t.Fatal(err)
	}
	stored := models.ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(real)
	upgraded := mustLLMCodec(t, llmWithKeys(reference, "hub-secret", stored, &exp), now)
	if id, err := upgraded.Unwrap(rt, genuine, victim); err != nil || id != "msgbatch_01app1" {
		t.Fatalf("a genuine pre-upgrade id = %q, %v", id, err)
	}
	if _, err := upgraded.Unwrap(rt, forged, victim); err == nil {
		t.Fatalf("forged id accepted next to a genuine previous key: %v", err)
	}
}
