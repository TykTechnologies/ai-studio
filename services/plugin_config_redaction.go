package services

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// Redaction of secret plugin configuration fields.
//
// A plugin's configuration is a free-form JSON object the plugin describes
// with a JSON Schema (ConfigProvider). The management API returns it in many
// responses, and anyone who may read the plugin could read a Redis password
// or a vault token stored in it. So the API never returns the value of a
// secret field: RedactPluginConfig replaces it with REDACTED_VALUE on the way
// out, and RestorePluginConfig puts the stored value back where a write sends
// the marker in, as the other credential fields of the API do. The stored
// configuration, what the plugin process receives and what is synced to
// edges are never touched.
//
// Which fields are secret:
//
//   - With a schema: every position the schema marks format "password" or
//     writeOnly true, the two markers the edit form masks
//     (SchemaFormRenderer.isSecretProperty), at any depth: nested objects,
//     array items, additionalProperties, patternProperties, local $refs and the branches of
//     allOf, anyOf, oneOf and if/then/else. A position is secret when any
//     applicable branch marks it.
//   - Without a usable schema (the plugin declares none, it was never
//     fetched, or it does not parse): by key name. A string under a key
//     that names a credential (isSecretKeyName) is secret. This keeps a
//     plugin whose schema is not cached yet from being served in clear.
//
// A $SECRET/ or $ENV/ reference is not a secret value: it is returned as the
// reference. Empty values are returned as they are.

// pluginConfigMaxDepth bounds the walk of a configuration value.
const pluginConfigMaxDepth = 64

// pluginSchemaMaxExpansion bounds how many subschemas one position expands
// to ($ref hops and combinator branches together).
const pluginSchemaMaxExpansion = 64

// PluginConfigSecrets classifies the positions of a plugin configuration.
// The zero value (and nil) classify by key name.
type PluginConfigSecrets struct {
	root map[string]interface{}
	// patterns memoises the compiled patternProperties expressions (nil: a
	// pattern that does not compile). One classifier serves one request, so
	// it is not shared between goroutines.
	patterns map[string]*regexp.Regexp
}

// patternMatches reports whether key matches a patternProperties pattern. A
// pattern that does not compile counts as matching.
func (s *PluginConfigSecrets) patternMatches(pattern, key string) bool {
	re, seen := s.patterns[pattern]
	if !seen {
		re, _ = regexp.Compile(pattern)
		if s.patterns == nil {
			s.patterns = map[string]*regexp.Regexp{}
		}
		s.patterns[pattern] = re
	}
	return re == nil || re.MatchString(key)
}

// NewPluginConfigSecrets builds the classifier for a plugin's configuration
// schema. An empty or unparseable schema, or one that is not an object,
// gives the key-name classifier.
func NewPluginConfigSecrets(schemaJSON string) *PluginConfigSecrets {
	if strings.TrimSpace(schemaJSON) == "" {
		return &PluginConfigSecrets{}
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(schemaJSON), &root); err != nil || len(root) == 0 {
		return &PluginConfigSecrets{}
	}
	return &PluginConfigSecrets{root: root}
}

// BySchema reports whether the classifier reads a schema (false: key names).
func (s *PluginConfigSecrets) BySchema() bool { return s != nil && s.root != nil }

// secretKeyNameParts are the words that mark a key as holding a credential
// when no schema says so.
var secretKeyNameParts = []string{
	"password", "passwd", "secret", "token", "apikey", "api_key", "api-key",
	"privatekey", "private_key", "private-key", "credential", "authorization",
}

func isSecretKeyName(key string) bool {
	k := strings.ToLower(key)
	for _, part := range secretKeyNameParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

// position is where the walk stands: the subschemas that apply (schema
// mode) and the nearest object key (key-name mode).
type position struct {
	schemas []map[string]interface{}
	key     string
	// overflow: more subschemas apply here than the walk follows. The
	// position is then treated as secret rather than read incompletely.
	overflow bool
}

// bounded removes repeated subschemas and marks the position as overflowing
// when more than pluginSchemaMaxExpansion distinct ones remain, so a schema
// whose branches multiply at every level cannot make the walk grow with it.
func bounded(schemas []map[string]interface{}, key string) position {
	seen := make(map[uintptr]bool, len(schemas))
	out := schemas[:0:0]
	for _, m := range schemas {
		ptr := reflect.ValueOf(m).Pointer()
		if seen[ptr] {
			continue
		}
		seen[ptr] = true
		out = append(out, m)
	}
	if len(out) > pluginSchemaMaxExpansion {
		return position{key: key, overflow: true}
	}
	return position{schemas: out, key: key}
}

func (s *PluginConfigSecrets) rootPosition() position {
	if !s.BySchema() {
		return position{}
	}
	var c collector
	c.add(s, s.root)
	return c.position("")
}

// expand resolves schema to the subschemas that apply at its position: the
// schema itself with local $refs followed, and the branches of its
// combinators. It follows at most pluginSchemaMaxExpansion subschemas and
// reports whether it had to stop before following them all; the caller then
// treats the position as secret instead of trusting a partial reading.
func (s *PluginConfigSecrets) expand(schema map[string]interface{}) ([]map[string]interface{}, bool) {
	var out []map[string]interface{}
	budget := pluginSchemaMaxExpansion
	overflow := false
	var visit func(node map[string]interface{})
	visit = func(node map[string]interface{}) {
		for node != nil {
			if budget == 0 {
				overflow = true
				return
			}
			budget--
			out = append(out, node)
			for _, comb := range []string{"allOf", "anyOf", "oneOf"} {
				if branches, ok := node[comb].([]interface{}); ok {
					for _, b := range branches {
						if bm, ok := b.(map[string]interface{}); ok {
							visit(bm)
						}
					}
				}
			}
			for _, cond := range []string{"then", "else"} {
				if bm, ok := node[cond].(map[string]interface{}); ok {
					visit(bm)
				}
			}
			ref, ok := node["$ref"].(string)
			if !ok {
				return
			}
			node = s.resolveRef(ref)
		}
	}
	visit(schema)
	return out, overflow
}

// collect gathers the expansion of several subschemas into a position.
type collector struct {
	schemas  []map[string]interface{}
	overflow bool
}

func (c *collector) add(s *PluginConfigSecrets, schema map[string]interface{}) {
	expanded, overflow := s.expand(schema)
	c.schemas = append(c.schemas, expanded...)
	c.overflow = c.overflow || overflow
}

func (c *collector) position(key string) position {
	if c.overflow {
		return position{key: key, overflow: true}
	}
	return bounded(c.schemas, key)
}

// resolveRef follows a local JSON pointer ("#/definitions/X", "#/$defs/X").
// Remote and unresolvable refs give nil.
func (s *PluginConfigSecrets) resolveRef(ref string) map[string]interface{} {
	if ref == "#" {
		return s.root
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var node interface{} = s.root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		m, ok := node.(map[string]interface{})
		if !ok {
			return nil
		}
		node = m[part]
	}
	m, _ := node.(map[string]interface{})
	return m
}

// secret reports whether the position holds a secret.
func (s *PluginConfigSecrets) secret(p position) bool {
	if !s.BySchema() {
		return isSecretKeyName(p.key)
	}
	if p.overflow {
		return true
	}
	for _, schema := range p.schemas {
		if f, _ := schema["format"].(string); f == "password" {
			return true
		}
		if w, _ := schema["writeOnly"].(bool); w {
			return true
		}
	}
	return false
}

// child is the position of key inside an object at p: the key's own
// subschema, else every patternProperties subschema whose pattern matches
// the key (a pattern that does not compile counts as matching), else
// additionalProperties.
func (s *PluginConfigSecrets) child(p position, key string) position {
	if !s.BySchema() {
		return position{key: key}
	}
	if p.overflow {
		return position{key: key, overflow: true}
	}
	var c collector
	for _, schema := range p.schemas {
		matched := false
		if props, ok := schema["properties"].(map[string]interface{}); ok {
			if sub, ok := props[key].(map[string]interface{}); ok {
				c.add(s, sub)
				matched = true
			}
		}
		if patterns, ok := schema["patternProperties"].(map[string]interface{}); ok {
			for pattern, raw := range patterns {
				sub, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if s.patternMatches(pattern, key) {
					c.add(s, sub)
					matched = true
				}
			}
		}
		if matched {
			continue
		}
		if sub, ok := schema["additionalProperties"].(map[string]interface{}); ok {
			c.add(s, sub)
		}
	}
	return c.position(key)
}

// item is the position of element i of an array at p.
func (s *PluginConfigSecrets) item(p position, i int) position {
	if !s.BySchema() {
		return position{key: p.key}
	}
	if p.overflow {
		return position{key: p.key, overflow: true}
	}
	var c collector
	for _, schema := range p.schemas {
		switch items := schema["items"].(type) {
		case map[string]interface{}:
			c.add(s, items)
		case []interface{}:
			if i < len(items) {
				if sub, ok := items[i].(map[string]interface{}); ok {
					c.add(s, sub)
				}
			} else if sub, ok := schema["additionalItems"].(map[string]interface{}); ok {
				c.add(s, sub)
			}
		}
		if prefix, ok := schema["prefixItems"].([]interface{}); ok && i < len(prefix) {
			if sub, ok := prefix[i].(map[string]interface{}); ok {
				c.add(s, sub)
			}
		}
	}
	return c.position(p.key)
}

// redactable reports whether v, at a secret position, is a value to hide:
// anything but nothing, an empty string and a secret reference.
func redactable(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != "" && !secrets.IsReference(x)
	}
	return true
}

// RedactPluginConfig returns a copy of config with every secret value
// replaced by REDACTED_VALUE. config is not modified. A nil config stays
// nil.
func (s *PluginConfigSecrets) RedactPluginConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	out, _ := s.redact(config, s.rootPosition(), 0).(map[string]interface{})
	return out
}

func (s *PluginConfigSecrets) redact(v interface{}, p position, depth int) interface{} {
	if depth > pluginConfigMaxDepth {
		// Deeper than any real configuration: hide it rather than walk on.
		return REDACTED_VALUE
	}
	if depth > 0 && s.secret(p) {
		// In key-name mode only strings (and strings in lists) under the
		// key are hidden; an object under it is walked on.
		if _, isString := v.(string); isString || s.BySchema() {
			if redactable(v) {
				return REDACTED_VALUE
			}
			return v
		}
	}
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, val := range x {
			out[k] = s.redact(val, s.child(p, k), depth+1)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, val := range x {
			out[i] = s.redact(val, s.item(p, i), depth+1)
		}
		return out
	}
	return v
}

// PluginConfigMarkerError says a write used the redaction marker where the
// stored value cannot be kept. Path names the position; it never carries a
// submitted value.
type PluginConfigMarkerError struct {
	Path   string
	Reason string
}

func (e *PluginConfigMarkerError) Error() string {
	return fmt.Sprintf("config%s: %s", e.Path, e.Reason)
}

const (
	markerNotSecret    = "the value \"" + REDACTED_VALUE + "\" stands for a stored secret and is only accepted in a secret field; send the real value"
	markerListChanged  = "a list entry that keeps a stored secret (\"" + REDACTED_VALUE + "\") must be otherwise unchanged and in its place; send the secret again for entries you add, move or change"
	markerTooDeep      = "the configuration is nested too deeply"
	dropRestoredSecret = dropValue(0)
)

type dropValue int

// RestorePluginConfig returns incoming with every REDACTED_VALUE at a secret
// position replaced by the value stored at the same position, so a form that
// was loaded with redacted secrets and saved without retyping them keeps
// them. incoming and stored are not modified.
//
//   - The marker at a secret position with no stored value stores nothing
//     there: the literal marker is never persisted as a credential.
//   - The marker anywhere else is refused (PluginConfigMarkerError). Putting
//     it in a readable field must never copy a secret into that field.
//   - Inside a list, an entry that contains the marker must equal the
//     redacted form of the stored entry at the same index (and a list of
//     secret strings must keep its length). Otherwise deleting or reordering
//     entries would hand one entry's secret to another.
//
// A nil incoming stays nil.
func (s *PluginConfigSecrets) RestorePluginConfig(incoming, stored map[string]interface{}) (map[string]interface{}, error) {
	if incoming == nil {
		return nil, nil
	}
	out, err := s.restore(incoming, stored, s.rootPosition(), "", 0)
	if err != nil {
		return nil, err
	}
	m, _ := out.(map[string]interface{})
	return m, nil
}

func containsMarker(v interface{}) bool {
	switch x := v.(type) {
	case string:
		return x == REDACTED_VALUE
	case map[string]interface{}:
		for _, val := range x {
			if containsMarker(val) {
				return true
			}
		}
	case []interface{}:
		for _, val := range x {
			if containsMarker(val) {
				return true
			}
		}
	}
	return false
}

func (s *PluginConfigSecrets) restore(in, stored interface{}, p position, path string, depth int) (interface{}, error) {
	if depth > pluginConfigMaxDepth {
		return nil, &PluginConfigMarkerError{Path: path, Reason: markerTooDeep}
	}
	if str, ok := in.(string); ok && str == REDACTED_VALUE {
		if depth == 0 || !s.secret(p) {
			return nil, &PluginConfigMarkerError{Path: path, Reason: markerNotSecret}
		}
		if redactable(stored) {
			return stored, nil
		}
		return dropRestoredSecret, nil
	}
	if depth > 0 && s.BySchema() && s.secret(p) {
		// A new value for a secret position, whatever its shape: taken as
		// sent (a marker nested inside it has no stored counterpart).
		if containsMarker(in) {
			return nil, &PluginConfigMarkerError{Path: path, Reason: markerNotSecret}
		}
		return in, nil
	}
	switch x := in.(type) {
	case map[string]interface{}:
		storedMap, _ := stored.(map[string]interface{})
		out := make(map[string]interface{}, len(x))
		for k, val := range x {
			restored, err := s.restore(val, storedMap[k], s.child(p, k), path+"."+k, depth+1)
			if err != nil {
				return nil, err
			}
			if restored == dropRestoredSecret {
				continue
			}
			out[k] = restored
		}
		return out, nil
	case []interface{}:
		storedList, _ := stored.([]interface{})
		out := make([]interface{}, 0, len(x))
		for i, val := range x {
			ip := s.item(p, i)
			ipath := path + "[" + strconv.Itoa(i) + "]"
			if containsMarker(val) {
				if i >= len(storedList) {
					return nil, &PluginConfigMarkerError{Path: ipath, Reason: markerListChanged}
				}
				if _, scalar := val.(string); scalar && len(x) != len(storedList) {
					return nil, &PluginConfigMarkerError{Path: ipath, Reason: markerListChanged}
				}
				if !reflect.DeepEqual(val, s.redact(storedList[i], ip, depth+1)) {
					if s.secret(ip) || !isContainer(val) {
						return nil, &PluginConfigMarkerError{Path: ipath, Reason: markerNotSecretOrChanged(s.secret(ip))}
					}
					return nil, &PluginConfigMarkerError{Path: ipath, Reason: markerListChanged}
				}
				out = append(out, storedList[i])
				continue
			}
			restored, err := s.restore(val, nil, ip, ipath, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, restored)
		}
		return out, nil
	}
	return in, nil
}

func isContainer(v interface{}) bool {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		return true
	}
	return false
}

func markerNotSecretOrChanged(secret bool) string {
	if secret {
		return markerListChanged
	}
	return markerNotSecret
}
