package database

import (
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"
)

// GovernedMetadataContextKey is the plugin-context metadata key carrying the
// LLM's gateway-visible governed metadata as a JSON object string.
// Individual top-level values are also flattened as "governed_metadata.<key>"
// so plugins can read e.g. ctx.Metadata["governed_metadata.data_classification"]
// without parsing JSON.
const GovernedMetadataContextKey = "governed_metadata"

// GovernedMetadataJSON converts the snapshot's JSON string into a column value,
// keeping the column NULL when the control plane sent nothing.
func GovernedMetadataJSON(raw string) datatypes.JSON {
	if raw == "" {
		return nil
	}
	return datatypes.JSON(raw)
}

// AddGovernedMetadataToContext copies the LLM's governed metadata into a plugin
// context metadata map. It is a no-op for a nil LLM or empty metadata.
func AddGovernedMetadataToContext(meta map[string]interface{}, llm *LLM) {
	if meta == nil || llm == nil || len(llm.GovernedMetadata) == 0 {
		return
	}
	raw := string(llm.GovernedMetadata)
	meta[GovernedMetadataContextKey] = raw

	var values map[string]interface{}
	if err := json.Unmarshal(llm.GovernedMetadata, &values); err != nil {
		return
	}
	for k, v := range values {
		meta[GovernedMetadataContextKey+"."+k] = flattenGovernedValue(v)
	}
}

// flattenGovernedValue renders a value as a string suitable for the
// map<string,string> plugin context: scalars as-is, lists comma-joined,
// anything else as JSON.
func flattenGovernedValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return fmt.Sprintf("%t", t)
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case []interface{}:
		out := ""
		for i, item := range t {
			if i > 0 {
				out += ","
			}
			out += flattenGovernedValue(item)
		}
		return out
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
