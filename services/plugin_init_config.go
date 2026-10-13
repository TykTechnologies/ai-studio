package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/rs/zerolog/log"
)

// secretRefPrefix marks a plugin config value that names an AI Studio secret.
const secretRefPrefix = "$SECRET/"

// pluginInitConfig converts a plugin's stored config into the string map
// sent with Initialize. Complex values (arrays, objects) are JSON-encoded so
// plugins can parse them.
//
// A top-level string value that is exactly a secret reference, $SECRET/NAME,
// is resolved here, so a plugin can keep a credential out of its stored
// config. Only the Studio-side plugin process sees the value: the database
// and the configuration snapshot sent to edges keep the reference. Who may
// add a reference is checked where config is written (NewSecretReferences).
//
// $ENV/NAME is deliberately not resolved: AI Studio's environment holds its
// own secrets (encryption key, database URL), and a config writer must not be
// able to hand them to a plugin. A plugin may resolve $ENV/ itself against
// the environment it is given (the allowlist and PLUGIN_ENV_PASSTHROUGH).
func pluginInitConfig(config map[string]interface{}) map[string]string {
	configMap := make(map[string]string, len(config))
	for k, v := range config {
		switch val := v.(type) {
		case string:
			configMap[k] = resolveSecretReference(k, val)
		case int, int64, uint, uint64, float64, bool:
			configMap[k] = fmt.Sprintf("%v", val)
		default:
			jsonBytes, err := json.Marshal(val)
			if err != nil {
				log.Warn().
					Str("key", k).
					Err(err).
					Msg("Failed to JSON encode config value, using string representation")
				configMap[k] = fmt.Sprintf("%v", val)
			} else {
				configMap[k] = string(jsonBytes)
			}
		}
	}
	return configMap
}

// resolveSecretReference resolves value if it is a $SECRET/NAME reference and
// returns any other value unchanged. A reference that cannot be resolved is
// passed through as is, so the plugin can report it; no value is logged.
func resolveSecretReference(key, value string) string {
	if !strings.HasPrefix(value, secretRefPrefix) {
		return value
	}
	resolved := secrets.GetValue(value, false)
	if resolved == value {
		log.Warn().Str("key", key).Str("reference", value).Msg("Plugin config secret reference could not be resolved; passing it to the plugin unresolved")
	}
	return resolved
}

// isSecretReference reports whether v is a top-level secret reference string.
func isSecretReference(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.HasPrefix(s, secretRefPrefix)
}

// NewSecretReferences lists the keys of updated whose value is a secret
// reference that old does not already hold under the same key. Writing those
// needs the right to read secrets, since the plugin receives their values.
func NewSecretReferences(old, updated map[string]interface{}) []string {
	var keys []string
	for k, v := range updated {
		if isSecretReference(v) && old[k] != v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// RestoreSecretReferences returns updated with the references from old put
// back where a plugin wrote the resolved value of the reference it was given:
// a plugin that saves its in-memory config must not turn a reference into a
// stored plaintext secret.
func RestoreSecretReferences(old, updated map[string]interface{}) map[string]interface{} {
	for k, oldVal := range old {
		if !isSecretReference(oldVal) {
			continue
		}
		newVal, ok := updated[k].(string)
		if !ok || newVal == oldVal {
			continue
		}
		if resolved := secrets.GetValue(oldVal.(string), false); resolved != oldVal && newVal == resolved {
			updated[k] = oldVal
		}
	}
	return updated
}
