package services

import (
	"encoding/json"
	"strings"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/rs/zerolog/log"
)

// pluginConfigBytes is the config a plugin is initialised with: its stored
// config, with each top-level $SECRET/ reference replaced by the value the
// hub sent for it encrypted (database.Plugin.ConfigSecretsEncrypted). The
// stored config keeps the reference, so nothing that reads the plugin row
// (the admin API, the edge DB at rest) sees a value.
//
// Only a key whose stored value is a $SECRET/ reference is replaced. A value
// that is empty or does not decrypt becomes secrets.RefusedCredential, never
// an empty credential, the ciphertext or the reference. No value is logged.
func (a *PluginServiceAdapter) pluginConfigBytes(p database.Plugin) []byte {
	configBytes, _ := json.Marshal(p.Config)
	if len(p.ConfigSecretsEncrypted) == 0 {
		return configBytes
	}
	var encrypted map[string]string
	if err := json.Unmarshal([]byte(p.ConfigSecretsEncrypted), &encrypted); err != nil || len(encrypted) == 0 {
		return configBytes
	}
	// Raw values, so every other value goes to the plugin byte for byte
	// (a large integer would not survive a float64 round trip).
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil || cfg == nil {
		return configBytes
	}

	for key, ciphertext := range encrypted {
		var ref string
		if raw, ok := cfg[key]; !ok || json.Unmarshal(raw, &ref) != nil || !strings.HasPrefix(ref, secrets.SecretRefPrefix) {
			continue
		}
		value := secrets.RefusedCredential
		if ciphertext == "" {
			// The hub encrypts every value it sends, refusals included; an
			// empty field would decrypt to an empty credential.
			log.Error().Uint("plugin_id", p.ID).Str("key", key).Msg("Empty plugin config secret from the hub")
		} else if a.crypto == nil {
			log.Error().Uint("plugin_id", p.ID).Str("key", key).Msg("No crypto service to decrypt a plugin config secret")
		} else if plain, err := a.crypto.Decrypt(ciphertext); err != nil {
			log.Error().Err(err).Uint("plugin_id", p.ID).Str("key", key).Msg("Failed to decrypt a plugin config secret")
		} else {
			value = plain
		}
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		cfg[key] = raw
	}

	merged, err := json.Marshal(cfg)
	if err != nil {
		return configBytes
	}
	return merged
}
