package services

import (
	"encoding/json"
	"errors"
	"github.com/TykTechnologies/midsommar/v2/models"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const redactionTestSchema = `{
  "type": "object",
  "definitions": {
    "redis": {"type": "object", "properties": {
      "addr": {"type": "string"},
      "password": {"type": "string", "format": "password"}
    }},
    "node": {"type": "object", "properties": {
      "name": {"type": "string"},
      "key": {"type": "string", "writeOnly": true},
      "child": {"$ref": "#/definitions/node"}
    }}
  },
  "properties": {
    "redis_addr": {"type": "string"},
    "redis_password": {"type": "string", "format": "password"},
    "vault_token": {"type": "string", "writeOnly": true},
    "note": {"type": "string"},
    "backend": {"type": "object", "properties": {"redis": {"$ref": "#/definitions/redis"}}},
    "tokens": {"type": "array", "items": {"type": "string", "format": "password"}},
    "upstreams": {"type": "array", "items": {"type": "object", "properties": {
      "host": {"type": "string"},
      "password": {"type": "string", "format": "password"}
    }}},
    "headers": {"type": "object", "additionalProperties": {"type": "string", "writeOnly": true}},
    "auth": {"oneOf": [
      {"type": "object", "properties": {"kind": {"const": "basic"}, "pass": {"type": "string", "format": "password"}}},
      {"type": "object", "properties": {"kind": {"const": "none"}}}
    ]},
    "tls": {"allOf": [{"$ref": "#/definitions/redis"}, {"properties": {"cert": {"type": "string"}}}]},
    "tree": {"$ref": "#/definitions/node"},
    "blob": {"type": "object", "writeOnly": true}
  }
}`

func cfg(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

func asJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

const redactionTestConfig = `{
  "redis_addr": "redis:6379",
  "redis_password": "S3CRET-top",
  "vault_token": "S3CRET-vault",
  "note": "plain",
  "backend": {"redis": {"addr": "r2:6379", "password": "S3CRET-nested"}},
  "tokens": ["S3CRET-t0", "S3CRET-t1"],
  "upstreams": [{"host": "a", "password": "S3CRET-a"}, {"host": "b", "password": "S3CRET-b"}],
  "headers": {"Authorization": "S3CRET-hdr", "X-Other": "S3CRET-hdr2"},
  "auth": {"kind": "basic", "pass": "S3CRET-oneof"},
  "tls": {"addr": "t", "password": "S3CRET-allof", "cert": "pem"},
  "tree": {"name": "n0", "key": "S3CRET-k0", "child": {"name": "n1", "key": "S3CRET-k1", "child": {"name": "n2", "key": "S3CRET-k2"}}},
  "blob": {"anything": "S3CRET-blob"}
}`

func TestRedactPluginConfig_SchemaMarkedFieldsAtEveryDepth(t *testing.T) {
	s := NewPluginConfigSecrets(redactionTestSchema)
	require.True(t, s.BySchema())
	in := cfg(t, redactionTestConfig)
	out := s.RedactPluginConfig(in)

	js := asJSON(t, out)
	assert.NotContains(t, js, "S3CRET", "no secret value is returned")
	assert.JSONEq(t, redactionTestConfig, asJSON(t, in), "the stored configuration is not modified")

	assert.Equal(t, "redis:6379", out["redis_addr"])
	assert.Equal(t, "plain", out["note"])
	assert.Equal(t, REDACTED_VALUE, out["redis_password"])
	assert.Equal(t, REDACTED_VALUE, out["vault_token"], "writeOnly is a secret too")
	assert.Equal(t, map[string]interface{}{"addr": "r2:6379", "password": REDACTED_VALUE}, out["backend"].(map[string]interface{})["redis"])
	assert.Equal(t, []interface{}{REDACTED_VALUE, REDACTED_VALUE}, out["tokens"])
	assert.Equal(t, map[string]interface{}{"host": "b", "password": REDACTED_VALUE}, out["upstreams"].([]interface{})[1])
	assert.Equal(t, map[string]interface{}{"Authorization": REDACTED_VALUE, "X-Other": REDACTED_VALUE}, out["headers"])
	assert.Equal(t, map[string]interface{}{"kind": "basic", "pass": REDACTED_VALUE}, out["auth"])
	assert.Equal(t, map[string]interface{}{"addr": "t", "password": REDACTED_VALUE, "cert": "pem"}, out["tls"])
	assert.Equal(t, REDACTED_VALUE, out["blob"], "a secret object is hidden whole")
	tree := out["tree"].(map[string]interface{})
	assert.Equal(t, "n0", tree["name"])
	assert.Equal(t, "n2", tree["child"].(map[string]interface{})["child"].(map[string]interface{})["name"], "recursive definitions are followed with the value")
}

func TestRedactPluginConfig_ReferencesEmptyAndUnmarked(t *testing.T) {
	s := NewPluginConfigSecrets(redactionTestSchema)
	out := s.RedactPluginConfig(cfg(t, `{"redis_password":"$SECRET/REDIS_PW","vault_token":"$ENV/VAULT","tokens":["","$SECRET/T"],"backend":{"redis":{"password":""}}}`))
	assert.JSONEq(t, `{"redis_password":"$SECRET/REDIS_PW","vault_token":"$ENV/VAULT","tokens":["","$SECRET/T"],"backend":{"redis":{"password":""}}}`, asJSON(t, out),
		"a reference is shown as the reference; an empty value stays empty")

	plain := `{"type":"object","properties":{"url":{"type":"string"},"password_hint":{"type":"string"},"n":{"type":"integer"}}}`
	c := `{"url":"http://x","password_hint":"not marked secret","n":3,"extra":{"token":"also not marked"}}`
	assert.JSONEq(t, c, asJSON(t, NewPluginConfigSecrets(plain).RedactPluginConfig(cfg(t, c))),
		"a plugin whose schema marks no secret field is returned unchanged")

	assert.Nil(t, s.RedactPluginConfig(nil))
}

// Without a usable schema the configuration is not served in clear: fields
// whose key names a credential are hidden.
func TestRedactPluginConfig_WithoutSchemaFallsBackToKeyNames(t *testing.T) {
	c := `{"redis_password":"S3CRET-1","vaultToken":"S3CRET-2","API_KEY":"S3CRET-3","url":"http://x","count":2,
		"backend":{"redis":{"addr":"r","password":"S3CRET-4"}},"tokens":["S3CRET-5"],"secret_ref":"$SECRET/X","client_secret":""}`
	for name, schema := range map[string]string{"none": "", "blank": "  ", "not json": "{nope", "not an object": `["a"]`, "empty object": `{}`} {
		t.Run(name, func(t *testing.T) {
			s := NewPluginConfigSecrets(schema)
			require.False(t, s.BySchema())
			out := s.RedactPluginConfig(cfg(t, c))
			js := asJSON(t, out)
			assert.NotContains(t, js, "S3CRET")
			assert.Equal(t, "http://x", out["url"])
			assert.Equal(t, "$SECRET/X", out["secret_ref"])
			assert.Equal(t, "r", out["backend"].(map[string]interface{})["redis"].(map[string]interface{})["addr"])
		})
	}
	var nilSecrets *PluginConfigSecrets
	assert.NotContains(t, asJSON(t, nilSecrets.RedactPluginConfig(cfg(t, c))), "S3CRET")
}

func TestRestorePluginConfig_MarkerKeepsTheStoredValue(t *testing.T) {
	s := NewPluginConfigSecrets(redactionTestSchema)
	stored := cfg(t, redactionTestConfig)

	// The form as loaded, saved unchanged: everything comes back.
	roundTrip, err := s.RestorePluginConfig(s.RedactPluginConfig(stored), stored)
	require.NoError(t, err)
	assert.JSONEq(t, redactionTestConfig, asJSON(t, roundTrip))

	// A sibling changes, one secret is replaced, one is cleared, one key is dropped.
	in := s.RedactPluginConfig(stored)
	in["redis_addr"] = "other:6379"
	in["vault_token"] = "NEW-token"
	in["backend"].(map[string]interface{})["redis"].(map[string]interface{})["password"] = ""
	delete(in, "tls")
	out, err := s.RestorePluginConfig(in, stored)
	require.NoError(t, err)
	assert.Equal(t, "other:6379", out["redis_addr"])
	assert.Equal(t, "S3CRET-top", out["redis_password"], "kept")
	assert.Equal(t, "NEW-token", out["vault_token"], "replaced")
	assert.Equal(t, "", out["backend"].(map[string]interface{})["redis"].(map[string]interface{})["password"], "cleared")
	assert.NotContains(t, out, "tls", "dropped")
	assert.NotContains(t, asJSON(t, out), REDACTED_VALUE, "the marker is never stored")

	nilOut, err := s.RestorePluginConfig(nil, stored)
	require.NoError(t, err)
	assert.Nil(t, nilOut)
}

func TestRestorePluginConfig_MarkerWithNothingStoredStoresNothing(t *testing.T) {
	s := NewPluginConfigSecrets(redactionTestSchema)
	out, err := s.RestorePluginConfig(cfg(t, `{"redis_addr":"r","redis_password":"[redacted]","backend":{"redis":{"password":"[redacted]"}}}`), nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"redis_addr":"r","backend":{"redis":{}}}`, asJSON(t, out))

	// A stored reference or empty value is not a stored secret either.
	out, err = s.RestorePluginConfig(cfg(t, `{"redis_password":"[redacted]"}`), cfg(t, `{"redis_password":""}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, asJSON(t, out))
}

func markerErr(t *testing.T, err error) *PluginConfigMarkerError {
	t.Helper()
	var me *PluginConfigMarkerError
	require.True(t, errors.As(err, &me), "want a PluginConfigMarkerError, got %v", err)
	return me
}

// The marker must never copy a secret into a field that can be read back.
func TestRestorePluginConfig_MarkerOutsideASecretFieldIsRefused(t *testing.T) {
	s := NewPluginConfigSecrets(redactionTestSchema)
	stored := cfg(t, redactionTestConfig)
	for name, in := range map[string]string{
		"readable sibling":           `{"note":"[redacted]"}`,
		"unknown key":                `{"redis_password_copy":"[redacted]"}`,
		"readable nested field":      `{"backend":{"redis":{"addr":"[redacted]"}}}`,
		"inside a new secret object": `{"blob":{"x":"[redacted]"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.RestorePluginConfig(cfg(t, in), stored)
			me := markerErr(t, err)
			assert.NotContains(t, me.Error(), "S3CRET")
		})
	}
}

// Deleting, reordering or editing list entries must not hand one entry's
// secret to another.
func TestRestorePluginConfig_ListsKeepSecretsOnlyInPlace(t *testing.T) {
	s := NewPluginConfigSecrets(redactionTestSchema)
	stored := cfg(t, redactionTestConfig)

	ok := func(in string) map[string]interface{} {
		out, err := s.RestorePluginConfig(cfg(t, in), stored)
		require.NoError(t, err)
		return out
	}
	refused := func(in string) {
		_, err := s.RestorePluginConfig(cfg(t, in), stored)
		me := markerErr(t, err)
		assert.NotContains(t, me.Error(), "S3CRET")
		assert.True(t, strings.Contains(me.Path, "["), "the error names the list entry: %s", me.Path)
	}

	// Unchanged entries keep theirs; an entry appended with its own secret is fine.
	out := ok(`{"upstreams":[{"host":"a","password":"[redacted]"},{"host":"b","password":"[redacted]"},{"host":"c","password":"new-c"}]}`)
	assert.JSONEq(t, `[{"host":"a","password":"S3CRET-a"},{"host":"b","password":"S3CRET-b"},{"host":"c","password":"new-c"}]`, asJSON(t, out["upstreams"]))
	out = ok(`{"tokens":["[redacted]","replaced"]}`)
	assert.JSONEq(t, `["S3CRET-t0","replaced"]`, asJSON(t, out["tokens"]))

	// Entry 0 deleted: entry "b" now sits at index 0 and would get a's password.
	refused(`{"upstreams":[{"host":"b","password":"[redacted]"}]}`)
	// The host of an entry changed while its password is kept.
	refused(`{"upstreams":[{"host":"evil.example","password":"[redacted]"},{"host":"b","password":"[redacted]"}]}`)
	// An entry added with the marker has no stored secret.
	refused(`{"upstreams":[{"host":"a","password":"[redacted]"},{"host":"b","password":"[redacted]"},{"host":"c","password":"[redacted]"}]}`)
	// A list of secret strings that changed length.
	refused(`{"tokens":["[redacted]"]}`)
	refused(`{"tokens":["[redacted]","[redacted]","[redacted]"]}`)
}

func TestRestorePluginConfig_KeyNameModeAndDepth(t *testing.T) {
	s := NewPluginConfigSecrets("")
	stored := cfg(t, `{"redis_password":"S3CRET","url":"u"}`)
	out, err := s.RestorePluginConfig(cfg(t, `{"redis_password":"[redacted]","url":"v"}`), stored)
	require.NoError(t, err)
	assert.JSONEq(t, `{"redis_password":"S3CRET","url":"v"}`, asJSON(t, out))
	_, err = s.RestorePluginConfig(cfg(t, `{"url":"[redacted]"}`), stored)
	markerErr(t, err)

	deep := map[string]interface{}{}
	cur := deep
	for i := 0; i < pluginConfigMaxDepth+5; i++ {
		next := map[string]interface{}{}
		cur["n"] = next
		cur = next
	}
	cur["password"] = "S3CRET"
	assert.NotContains(t, asJSON(t, s.RedactPluginConfig(deep)), "S3CRET")
	_, err = s.RestorePluginConfig(deep, nil)
	markerErr(t, err)
}

// A schema that refers to itself in every branch must not blow up.
func TestPluginConfigSecrets_SelfReferencingSchemaIsBounded(t *testing.T) {
	schema := `{"$ref":"#","allOf":[{"$ref":"#"},{"$ref":"#"}],"anyOf":[{"$ref":"#"},{"$ref":"#"}],
		"properties":{"password":{"type":"string","format":"password"},"next":{"$ref":"#"}}}`
	s := NewPluginConfigSecrets(schema)
	out := s.RedactPluginConfig(cfg(t, `{"password":"S3CRET","next":{"password":"S3CRET","next":{"password":"S3CRET"}}}`))
	assert.NotContains(t, asJSON(t, out), "S3CRET")
}

// The plugin a system.plugin.* event carries has its secrets redacted: the
// payload reaches every subscribed plugin and the cluster event log.
func TestSystemEvents_PluginPayloadIsRedacted(t *testing.T) {
	plugin := &models.Plugin{Name: "rl", Command: "file:///rl", Config: cfg(t, redactionTestConfig)}
	plugin.ID = 4

	withSchema := &SystemEventEmitter{}
	withSchema.SetPluginSecrets(func(commands []string) map[string]*PluginConfigSecrets {
		require.Equal(t, []string{"file:///rl"}, commands)
		return map[string]*PluginConfigSecrets{"file:///rl": NewPluginConfigSecrets(redactionTestSchema)}
	})
	for name, e := range map[string]*SystemEventEmitter{"schema": withSchema, "no source": {}, "nil emitter": nil} {
		t.Run(name, func(t *testing.T) {
			for _, in := range []interface{}{plugin, *plugin} {
				out, ok := e.redactPluginForEvent(in).(*models.Plugin)
				require.True(t, ok)
				assert.Equal(t, uint(4), out.ID)
				assert.Equal(t, "redis:6379", out.Config["redis_addr"])
				assert.Equal(t, REDACTED_VALUE, out.Config["redis_password"])
				if name == "schema" {
					assert.NotContains(t, asJSON(t, out.Config), "S3CRET")
				}
			}
			assert.Equal(t, "S3CRET-top", plugin.Config["redis_password"], "the plugin itself is not modified")
		})
	}
	assert.Equal(t, "other", withSchema.redactPluginForEvent("other"))
	assert.Nil(t, withSchema.redactPluginForEvent(nil))
}

func TestRedactPluginConfig_PatternProperties(t *testing.T) {
	schema := `{"type":"object","properties":{"plain":{"type":"string"}},
		"patternProperties":{"^engine_.*_token$":{"type":"string","format":"password"},"(unclosed":{"type":"string","writeOnly":true}},
		"additionalProperties":{"type":"string"}}`
	s := NewPluginConfigSecrets(schema)
	out := s.RedactPluginConfig(cfg(t, `{"plain":"p","engine_a_token":"S3CRET-a","engine_a_url":"http://e","other":"S3CRET-unparsed-pattern"}`))
	assert.Equal(t, REDACTED_VALUE, out["engine_a_token"])
	// A pattern that does not compile counts as matching every key (as a
	// pattern applies to declared properties too): hidden, not guessed at.
	assert.Equal(t, REDACTED_VALUE, out["plain"])
	assert.Equal(t, REDACTED_VALUE, out["engine_a_url"])
	assert.Equal(t, REDACTED_VALUE, out["other"])

	ok := NewPluginConfigSecrets(`{"type":"object","patternProperties":{"_token$":{"type":"string","format":"password"}}}`)
	out = ok.RedactPluginConfig(cfg(t, `{"a_token":"S3CRET","a_url":"u"}`))
	assert.Equal(t, map[string]interface{}{"a_token": REDACTED_VALUE, "a_url": "u"}, out)
	restored, err := ok.RestorePluginConfig(out, cfg(t, `{"a_token":"S3CRET","a_url":"u"}`))
	require.NoError(t, err)
	assert.Equal(t, "S3CRET", restored["a_token"])
}

// A position with more subschemas than the walk follows is treated as
// secret: a marker in a branch that was not reached must not be missed.
func TestRedactPluginConfig_ExpansionOverflowFailsClosed(t *testing.T) {
	branches := make([]string, 0, pluginSchemaMaxExpansion+20)
	for i := 0; i < pluginSchemaMaxExpansion+19; i++ {
		branches = append(branches, `{"type":"string"}`)
	}
	branches = append(branches, `{"type":"string","format":"password"}`) // the last one, past the budget
	schema := `{"type":"object","properties":{"host":{"type":"string"},"cred":{"anyOf":[` + strings.Join(branches, ",") + `]}}}`
	s := NewPluginConfigSecrets(schema)
	out := s.RedactPluginConfig(cfg(t, `{"host":"h","cred":"S3CRET"}`))
	assert.Equal(t, map[string]interface{}{"host": "h", "cred": REDACTED_VALUE}, out)
}

// system.llm.* events (and LLM object hooks) carry the LLM with its plugins:
// the plugins are listed without their configuration.
func TestSystemEvents_LLMPayloadCarriesNoPluginConfig(t *testing.T) {
	plugin := &models.Plugin{Name: "rl", Command: "file:///rl", Config: cfg(t, redactionTestConfig)}
	plugin.ID = 4
	llm := &models.LLM{Name: "gpt", APIKey: "sk-live", Plugins: []*models.Plugin{plugin, nil}}
	for _, in := range []interface{}{llm, *llm} {
		js := asJSON(t, redactForEvent(in))
		assert.NotContains(t, js, "S3CRET")
		assert.NotContains(t, js, "redis:6379", "no plugin configuration at all")
		assert.NotContains(t, js, "sk-live")
		assert.Contains(t, js, `"name":"rl"`, "the plugin is still listed")
	}
	assert.Equal(t, "S3CRET-top", plugin.Config["redis_password"], "the stored plugin is not modified")
	assert.Same(t, plugin, llm.Plugins[0])
}
