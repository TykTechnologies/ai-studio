package guardrails

import (
	"errors"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails/internal/v222"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// Obviously fake: never a real key in tests.
const fakeGuardrailKey = "fake-guardrail-provider-key"

func secretConnectionRaw() map[string]any {
	return map[string]any{
		"provider":   "azure_content_safety",
		"connection": map[string]any{"endpoint": "https://cs.example", "api_key": "$SECRET/CS_KEY"},
		"detectors":  []any{map[string]any{"name": "hate"}},
		"on_detect":  ActionBlock,
	}
}

func resolveFake(v string) string {
	if v == "$SECRET/CS_KEY" {
		return fakeGuardrailKey
	}
	return v
}

// fakeEncrypt and fakeDecrypt stand in for the edge encryption key.
func fakeEncrypt(v string) string { return "enc:" + strings.ToUpper(v) }
func fakeDecrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("not ours")
	}
	return strings.ToLower(strings.TrimPrefix(c, "enc:")), nil
}

// The edge snapshot never carries a resolved secret connection value in
// clear: it travels encrypted in connection_encrypted, the connection field
// holds the refused key, and the edge decrypts it back where it loads the
// config (vgi).
func TestConfigJSONForEdge_EncryptsSecretConnectionValues(t *testing.T) {
	js, err := ConfigJSONForEdge(secretConnectionRaw(), resolveFake, fakeEncrypt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(js, fakeGuardrailKey) {
		t.Fatalf("the snapshot carries the key in clear: %s", js)
	}
	raw, err := ConfigFromJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	if got := raw["connection"].(map[string]any)["api_key"]; got != secrets.RefusedCredential {
		t.Errorf("connection.api_key = %v, want the refused key", got)
	}

	if failed := DecryptEdgeConnection(raw, fakeDecrypt); len(failed) != 0 {
		t.Fatalf("failed: %v", failed)
	}
	if _, ok := raw[connectionEncryptedKey]; ok {
		t.Error("connection_encrypted is left in the config")
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connection["api_key"] != fakeGuardrailKey {
		t.Errorf("decrypted api_key = %q", cfg.Connection["api_key"])
	}
	if cfg.Connection["endpoint"] != "https://cs.example" {
		t.Errorf("endpoint = %q", cfg.Connection["endpoint"])
	}
}

// A value that does not decrypt, or a snapshot built without encryption,
// leaves the refused key: the provider refuses it and the fail mode applies.
func TestDecryptEdgeConnection_FailureKeepsRefusedKey(t *testing.T) {
	js, err := ConfigJSONForEdge(secretConnectionRaw(), resolveFake, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(js, fakeGuardrailKey) {
		t.Fatalf("an unencryptable key goes out in clear: %s", js)
	}

	js, err = ConfigJSONForEdge(secretConnectionRaw(), resolveFake, fakeEncrypt)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := ConfigFromJSON(js)
	failed := DecryptEdgeConnection(raw, func(string) (string, error) { return "", errors.New("wrong key") })
	if len(failed) != 1 || failed[0] != "api_key" {
		t.Fatalf("failed = %v", failed)
	}
	if got := raw["connection"].(map[string]any)["api_key"]; got != secrets.RefusedCredential {
		t.Errorf("connection.api_key = %v, want the refused key", got)
	}
}

// A v2.2.2 edge ignores connection_encrypted and reads the refused key, never
// the ciphertext, as its api_key.
func TestEdgeSnapshot_OldEdgeGetsRefusedGuardrailKey(t *testing.T) {
	js, err := ConfigJSONForEdge(secretConnectionRaw(), resolveFake, fakeEncrypt)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := v222.ConfigFromJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	old, err := v222.ParseConfig(raw)
	if err != nil {
		t.Fatalf("a v2.2.2 edge cannot parse the snapshot: %v", err)
	}
	if old.Connection["api_key"] != secrets.RefusedCredential {
		t.Errorf("a v2.2.2 edge uses %q as the key, want the refused key", old.Connection["api_key"])
	}
}
