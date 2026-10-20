package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createdID reads data.id (a string or a number) from a JSON:API response.
func createdID(t *testing.T, body []byte) uint64 {
	t.Helper()
	var resp struct {
		Data struct {
			ID json.RawMessage `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp), string(body))
	id, err := strconv.ParseUint(strings.Trim(string(resp.Data.ID), `"`), 10, 64)
	require.NoError(t, err, string(body))
	return id
}

func withAttrs(body map[string]interface{}, kv map[string]interface{}) map[string]interface{} {
	attrs := body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	for k, v := range kv {
		attrs[k] = v
	}
	return body
}

// jyx.99: every tracing field on LLMs, tools and datasources needs
// traces:write to change; the resource's own write permission is not
// enough. Leaving a field as stored needs nothing more.
func TestTracingFieldsNeedTracesWrite(t *testing.T) {
	a, _ := setupTestAPI(t)
	if a.config != nil {
		testMode := a.config.TestMode
		a.config.TestMode = false
		defer func() { a.config.TestMode = testMode }()
	}
	writes := []authz.Permission{authz.Write("llms"), authz.Write("tools"), authz.Write("datasources"),
		authz.Publish("llms"), authz.Publish("tools"), authz.Publish("datasources")}
	editor := credentialRouter(a, authz.NewSet(writes...))
	tracer := credentialRouter(a, authz.NewSet(append(writes, authz.Write("traces"))...))

	type obj struct {
		path string
		body func(name string) map[string]interface{}
		raw  map[string]interface{}
	}
	objs := []obj{
		{"/llms", func(n string) map[string]interface{} { return llmRefBody(n, "sk-plain", nil) },
			map[string]interface{}{"tracing_content_mode_max": "full"}},
		{"/llms", func(n string) map[string]interface{} { return llmRefBody(n, "sk-plain", nil) },
			map[string]interface{}{"tracing_propagation_opt_out": true}},
		{"/tools", slugToolBody, map[string]interface{}{"tracing_content_mode_max": "full"}},
		{"/datasources", func(n string) map[string]interface{} { return dsRefBody(n, "plain-key") },
			map[string]interface{}{"tracing_content_mode_max": "full"}},
	}
	for i, o := range objs {
		name := fmt.Sprintf("gate-%d", i)
		w := performRequest(editor, "POST", o.path, withAttrs(o.body(name), o.raw))
		assert.Equal(t, http.StatusForbidden, w.Code, "create %s %v without traces:write: %s", o.path, o.raw, w.Body.String())
		assert.Contains(t, w.Body.String(), "traces:write")

		w = performRequest(tracer, "POST", o.path, withAttrs(o.body(name), o.raw))
		require.Less(t, w.Code, 300, "create %s with traces:write: %s", o.path, w.Body.String())
		id := createdID(t, w.Body.Bytes())

		w = performRequest(editor, "PATCH", fmt.Sprintf("%s/%d", o.path, id), withAttrs(o.body(name), o.raw))
		assert.Less(t, w.Code, 300, "an unchanged field needs no traces:write (%s): %s", o.path, w.Body.String())

		lower := map[string]interface{}{}
		for k, v := range o.raw {
			if _, isBool := v.(bool); isBool {
				lower[k] = false
			} else {
				lower[k] = "metadata"
			}
		}
		w = performRequest(editor, "PATCH", fmt.Sprintf("%s/%d", o.path, id), withAttrs(o.body(name), lower))
		assert.Equal(t, http.StatusForbidden, w.Code, "changing %s %v without traces:write: %s", o.path, lower, w.Body.String())

		w = performRequest(tracer, "PATCH", fmt.Sprintf("%s/%d", o.path, id), withAttrs(o.body(name), lower))
		assert.Less(t, w.Code, 300, "changing %s with traces:write: %s", o.path, w.Body.String())
	}
}
