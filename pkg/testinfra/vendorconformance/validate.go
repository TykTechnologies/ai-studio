package vendorconformance

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"
)

//go:embed schema/*.json
var schemaFS embed.FS

// Format names a canonical wire schema.
type Format string

const (
	FormatOpenAICompletion     Format = "openai_chat_completion"
	FormatOpenAIChunk          Format = "openai_chat_completion_chunk"
	FormatOpenAIError          Format = "openai_error"
	FormatOpenAIModelsList     Format = "openai_models_list"
	FormatAnthropicMessage     Format = "anthropic_message"
	FormatAnthropicStreamEvent Format = "anthropic_stream_event"
	FormatGeminiGenerate       Format = "gemini_generate_content"
)

var (
	schemaOnce  sync.Once
	schemaCache map[Format]*gojsonschema.Schema
	schemaRaw   map[Format]map[string]any
	schemaErr   error
)

func loadSchemas() {
	schemaCache = map[Format]*gojsonschema.Schema{}
	schemaRaw = map[Format]map[string]any{}

	entries, err := schemaFS.ReadDir("schema")
	if err != nil {
		schemaErr = err
		return
	}
	for _, e := range entries {
		data, err := schemaFS.ReadFile("schema/" + e.Name())
		if err != nil {
			schemaErr = fmt.Errorf("reading %s: %w", e.Name(), err)
			return
		}
		format := Format(strings.TrimSuffix(e.Name(), ".json"))

		compiled, err := gojsonschema.NewSchema(gojsonschema.NewBytesLoader(data))
		if err != nil {
			schemaErr = fmt.Errorf("compiling %s: %w", e.Name(), err)
			return
		}
		schemaCache[format] = compiled

		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			schemaErr = fmt.Errorf("parsing %s: %w", e.Name(), err)
			return
		}
		schemaRaw[format] = raw
	}
}

func schemaFor(f Format) (*gojsonschema.Schema, map[string]any, error) {
	schemaOnce.Do(loadSchemas)
	if schemaErr != nil {
		return nil, nil, schemaErr
	}
	s, ok := schemaCache[f]
	if !ok {
		return nil, nil, fmt.Errorf("no schema for format %q", f)
	}
	return s, schemaRaw[f], nil
}

// Violation is one place a payload failed its canonical schema.
type Violation struct {
	Pointer string // JSON pointer-ish path, e.g. "choices.0.message.tool_calls.0.function.arguments"
	Message string
}

func (v Violation) String() string {
	where := v.Pointer
	if where == "" {
		where = "(root)"
	}
	return fmt.Sprintf("%s: %s", where, v.Message)
}

// Drift is one field present in a payload but absent from the canonical schema.
// Drift is the early-warning signal for upstream changes: a vendor adding a
// field shows up here before it becomes a translation bug.
type Drift struct {
	Pointer string
	Kind    string // JSON type of the undocumented value
}

// Result is the outcome of validating one payload.
type Result struct {
	Format     Format
	Violations []Violation
	Drift      []Drift
}

// OK reports whether the payload satisfied its schema. Drift alone is not a
// failure unless the caller is running in strict mode.
func (r Result) OK() bool { return len(r.Violations) == 0 }

// Error renders violations as a single multi-line message, or "" if clean.
func (r Result) Error() string {
	if len(r.Violations) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d schema violation(s) against %s:", len(r.Violations), r.Format)
	for _, v := range r.Violations {
		b.WriteString("\n  - " + v.String())
	}
	return b.String()
}

// Validate checks a raw JSON payload against a canonical schema and records any
// undocumented fields as drift.
func Validate(f Format, payload []byte) (Result, error) {
	schema, raw, err := schemaFor(f)
	if err != nil {
		return Result{}, err
	}

	res := Result{Format: f}

	loaded, err := schema.Validate(gojsonschema.NewBytesLoader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("validating against %s: %w", f, err)
	}
	for _, e := range loaded.Errors() {
		res.Violations = append(res.Violations, Violation{
			Pointer: e.Field(),
			Message: e.Description(),
		})
	}

	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return Result{}, fmt.Errorf("payload is not valid JSON: %w", err)
	}
	res.Drift = findDrift(raw, decoded, "")
	sort.Slice(res.Drift, func(i, j int) bool { return res.Drift[i].Pointer < res.Drift[j].Pointer })

	return res, nil
}

// findDrift walks a decoded payload alongside its schema, collecting every path
// the schema does not describe.
//
// It descends only where the schema descends: an object whose schema has no
// "properties" (a free-form map like Anthropic's tool `input`) is opaque, and
// its contents are neither validated nor reported. That is deliberate — user
// tool arguments are data, not contract.
func findDrift(schema map[string]any, value any, path string) []Drift {
	if schema == nil {
		return nil
	}

	// Unwrap a schema that is only a composition wrapper.
	schema = resolveComposite(schema, value)

	switch v := value.(type) {
	case map[string]any:
		props, hasProps := schema["properties"].(map[string]any)
		if !hasProps {
			// Opaque object by design (no declared properties). Not drift.
			return nil
		}
		var out []Drift
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			child := join(path, k)
			propSchema, known := props[k].(map[string]any)
			if !known {
				if allowsAdditional(schema) {
					continue // explicitly free-form; not a contract violation
				}
				out = append(out, Drift{Pointer: child, Kind: jsonKind(v[k])})
				continue
			}
			out = append(out, findDrift(propSchema, v[k], child)...)
		}
		return out

	case []any:
		itemSchema, ok := schema["items"].(map[string]any)
		if !ok {
			return nil
		}
		var out []Drift
		for i, item := range v {
			out = append(out, findDrift(itemSchema, item, fmt.Sprintf("%s.%d", path, i))...)
		}
		return out
	}

	return nil
}

// resolveComposite picks the branch of a oneOf/anyOf/allOf schema that best
// matches the value, so drift is measured against the branch actually in play
// rather than reported for every branch that does not apply.
func resolveComposite(schema map[string]any, value any) map[string]any {
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		branches, ok := schema[key].([]any)
		if !ok {
			continue
		}
		merged := map[string]any{}
		for k, v := range schema {
			if k != key {
				merged[k] = v
			}
		}
		mergedProps := map[string]any{}
		if p, ok := merged["properties"].(map[string]any); ok {
			for k, v := range p {
				mergedProps[k] = v
			}
		}
		for _, b := range branches {
			bs, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if p, ok := bs["properties"].(map[string]any); ok {
				for k, v := range p {
					mergedProps[k] = v
				}
			}
		}
		if len(mergedProps) > 0 {
			merged["properties"] = mergedProps
		}
		schema = merged
	}
	return schema
}

// allowsAdditional reports whether a schema explicitly permits undeclared
// properties. Absent additionalProperties, JSON Schema permits them — but for
// drift purposes we treat silence as "undocumented", which is the entire point.
func allowsAdditional(schema map[string]any) bool {
	switch a := schema["additionalProperties"].(type) {
	case bool:
		return a
	case map[string]any:
		return true
	}
	return false
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func jsonKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}
