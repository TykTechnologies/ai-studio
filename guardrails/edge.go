package guardrails

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// connectionEncryptedKey is Config.ConnectionEncrypted's JSON key.
const connectionEncryptedKey = "connection_encrypted"

// ConfigJSONForEdge renders a stored guardrail config for the edge snapshot.
// Connection values are passed through resolve so `$SECRET/` and `$ENV/`
// references become the values an edge, which has no access to the hub's
// secret store, can use. This mirrors how LLM API keys reach edges. Edges
// never resolve references themselves, so a value resolve leaves unresolved
// goes out without its name (EdgeConnectionValue).
//
// With encrypt, each resolved secret connection value (isSecretConnectionField)
// travels encrypted in connection_encrypted and the connection field holds
// secrets.RefusedCredential, as an LLM's metadata credentials do: the
// snapshot never carries it in clear, and an edge too old to decrypt it
// sends a key the provider refuses, so the filter's fail mode applies. encrypt
// returns "" when it cannot encrypt, which leaves the refused key alone.
func ConfigJSONForEdge(raw map[string]any, resolve func(string) string, encrypt func(string) string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return "", err
	}
	if resolve != nil {
		for k, v := range cfg.Connection {
			cfg.Connection[k] = EdgeConnectionValue(cfg.Provider, k, resolve(v))
		}
	}
	if encrypt != nil {
		for k, v := range cfg.Connection {
			if v == "" || v == secrets.RefusedCredential || !isSecretConnectionField(cfg.Provider, k) {
				continue
			}
			cfg.Connection[k] = secrets.RefusedCredential
			if c := encrypt(v); c != "" {
				if cfg.ConnectionEncrypted == nil {
					cfg.ConnectionEncrypted = make(map[string]string)
				}
				cfg.ConnectionEncrypted[k] = c
			}
		}
	}
	// An edge older than this hub rejects an action it does not know. Send
	// it block instead, the strongest action every edge applies, and the
	// real action where only newer edges look. New keys elsewhere in the
	// config (quarantine, tool_results, budgets) are ignored by old edges,
	// which then behave as they always have.
	if !legacyActions[cfg.OnDetect] {
		cfg.OnDetectExt = cfg.OnDetect
		cfg.OnDetect = ActionBlock
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EdgeConnectionValue renders one resolved connection value for the edge
// snapshot. A value that is still a reference goes out as a dud key
// (secrets.ForEdgeCredential) for a field the provider marks Secret, or for a
// field it does not declare, because a provider client may replace an empty
// key with credentials from its own environment (the AWS default chain, for
// one). Other fields (endpoints, regions) go out as a bare "$ENV/"
// (secrets.ForEdge), which is empty on released edges.
func EdgeConnectionValue(provider, field, resolved string) string {
	if isSecretConnectionField(provider, field) {
		return secrets.ForEdgeCredential(resolved)
	}
	return secrets.ForEdge(resolved)
}

// isSecretConnectionField reports whether the provider marks field Secret;
// an unknown provider or field counts as secret.
func isSecretConnectionField(provider, field string) bool {
	spec, ok := ProviderSpec(provider)
	if !ok {
		return true
	}
	for _, f := range spec.ConnectionFields {
		if f.Name == field {
			return f.Secret
		}
	}
	return true
}

// DecryptEdgeConnection is the edge's side of ConfigJSONForEdge's
// encryption: each connection_encrypted value is decrypted into connection,
// and connection_encrypted is removed. cfg is the decoded snapshot form
// (ConfigFromJSON) and is changed in place. A value that does not decrypt
// keeps the refused key in connection, so the provider refuses the call and
// the filter's fail mode applies; its field is returned in failed.
func DecryptEdgeConnection(cfg map[string]any, decrypt func(string) (string, error)) (failed []string) {
	enc, ok := cfg[connectionEncryptedKey].(map[string]any)
	delete(cfg, connectionEncryptedKey)
	if !ok || len(enc) == 0 {
		return nil
	}
	conn, _ := cfg["connection"].(map[string]any)
	if conn == nil {
		conn = make(map[string]any, len(enc))
		cfg["connection"] = conn
	}
	for k, v := range enc {
		s, _ := v.(string)
		plain, err := decrypt(s)
		if err != nil || s == "" {
			conn[k] = secrets.RefusedCredential
			failed = append(failed, k)
			continue
		}
		conn[k] = plain
	}
	return failed
}

// ConfigFromJSON decodes the edge snapshot form back into the stored map
// shape. An empty string is an absent config.
func ConfigFromJSON(s string) (map[string]any, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("guardrail config: %w", err)
	}
	return m, nil
}
