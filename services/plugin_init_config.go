package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/rs/zerolog/log"
)

// pluginInitConfig converts a plugin's stored config into the string map
// sent with Initialize. Complex values (arrays, objects) are JSON-encoded so
// plugins can parse them.
//
// A top-level string value that is exactly a reference, $SECRET/NAME or
// $ENV/NAME, is resolved here (secrets.GetValue), so a plugin can keep a
// token out of its stored config. Only the Studio-side plugin process sees
// the resolved value: the database and the configuration snapshot sent to
// edges keep the reference. $ENV/ resolves against AI Studio's own
// environment, which the operator controls, not the plugin's allowlisted one.
func pluginInitConfig(config map[string]interface{}) map[string]string {
	configMap := make(map[string]string, len(config))
	for k, v := range config {
		switch val := v.(type) {
		case string:
			configMap[k] = resolveConfigReference(k, val)
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

// resolveConfigReference resolves value if it is a $SECRET/NAME or $ENV/NAME
// reference, and returns any other value unchanged. A reference that cannot
// be resolved is passed through as is (secrets.GetValue logs why), so the
// plugin can report it; its value is never logged here.
func resolveConfigReference(key, value string) string {
	if !strings.HasPrefix(value, "$SECRET/") && !strings.HasPrefix(value, "$ENV/") {
		return value
	}
	resolved := secrets.GetValue(value, false)
	if resolved == value {
		log.Warn().Str("key", key).Str("reference", value).Msg("Plugin config reference could not be resolved; passing it to the plugin unresolved")
	}
	return resolved
}
