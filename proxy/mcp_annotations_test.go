package proxy

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPAnnotationsForMethod(t *testing.T) {
	cases := []struct {
		method                            string
		readOnly, destructive, idempotent bool
	}{
		{"GET", true, false, true},
		{"HEAD", true, false, true},
		{"OPTIONS", true, false, true},
		{"PUT", false, true, true},
		{"DELETE", false, true, true},
		{"POST", false, true, false},
		{"PATCH", false, true, false},
		// Unknown methods keep the cautious defaults.
		{"TRACE", false, true, false},
		{"", false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			a := mcpAnnotationsForMethod(tc.method)
			require.NotNil(t, a.ReadOnlyHint)
			require.NotNil(t, a.DestructiveHint)
			require.NotNil(t, a.IdempotentHint)
			require.NotNil(t, a.OpenWorldHint)

			assert.Equal(t, tc.readOnly, *a.ReadOnlyHint, "readOnlyHint")
			assert.Equal(t, tc.destructive, *a.DestructiveHint, "destructiveHint")
			assert.Equal(t, tc.idempotent, *a.IdempotentHint, "idempotentHint")
			// Every operation reaches a third-party API.
			assert.True(t, *a.OpenWorldHint, "openWorldHint")
		})
	}

	t.Run("method matching is case-insensitive", func(t *testing.T) {
		assert.True(t, *mcpAnnotationsForMethod("get").ReadOnlyHint)
	})
}

func TestApplyMCPAnnotationOverrides(t *testing.T) {
	t.Run("no extensions leaves the method-derived hints alone", func(t *testing.T) {
		a := applyMCPAnnotationOverrides(mcpAnnotationsForMethod("POST"), nil)
		assert.True(t, *a.DestructiveHint)
		assert.False(t, *a.ReadOnlyHint)
	})

	t.Run("an author can mark a POST read-only", func(t *testing.T) {
		// A POST that runs a search is not destructive; the HTTP method only
		// ever approximates what the operation does.
		a := applyMCPAnnotationOverrides(mcpAnnotationsForMethod("POST"), map[string]string{
			extReadOnlyHint:    "true",
			extDestructiveHint: "false",
			extIdempotentHint:  "true",
			extOpenWorldHint:   "false",
		})
		assert.True(t, *a.ReadOnlyHint)
		assert.False(t, *a.DestructiveHint)
		assert.True(t, *a.IdempotentHint)
		assert.False(t, *a.OpenWorldHint)
	})

	t.Run("a non-boolean value is ignored", func(t *testing.T) {
		a := applyMCPAnnotationOverrides(mcpAnnotationsForMethod("GET"), map[string]string{
			extDestructiveHint: "maybe",
		})
		assert.False(t, *a.DestructiveHint, "the method-derived hint must survive a bad override")
	})
}

func TestMCPAnnotationsForOperation(t *testing.T) {
	const spec = `{
      "openapi": "3.0.0",
      "info": {"title": "Currency Exchange Rates", "version": "1.0.0"},
      "servers": [{"url": "https://api.example.com"}],
      "paths": {
        "/rate/{base}/{quote}": {
          "get": {
            "operationId": "getExchangeRate",
            "parameters": [
              {"name": "base", "in": "path", "required": true, "schema": {"type": "string"}},
              {"name": "quote", "in": "path", "required": true, "schema": {"type": "string"}}
            ],
            "responses": {"200": {"description": "OK"}}
          }
        },
        "/alerts": {
          "post": {
            "operationId": "createAlert",
            "responses": {"201": {"description": "Created"}}
          }
        },
        "/search": {
          "post": {
            "operationId": "searchRates",
            "x-mcp-read-only": true,
            "x-mcp-destructive": false,
            "responses": {"200": {"description": "OK"}}
          }
        }
      }
    }`

	client, err := universalclient.NewClient([]byte(spec), "")
	require.NoError(t, err)

	t.Run("a read-only GET is not annotated destructive", func(t *testing.T) {
		a := mcpAnnotationsForOperation(client, "getExchangeRate")
		assert.True(t, *a.ReadOnlyHint)
		assert.False(t, *a.DestructiveHint, "a GET must not carry a DESTRUCTIVE badge")
		assert.True(t, *a.IdempotentHint)
		assert.True(t, *a.OpenWorldHint)
	})

	t.Run("a POST stays destructive", func(t *testing.T) {
		a := mcpAnnotationsForOperation(client, "createAlert")
		assert.False(t, *a.ReadOnlyHint)
		assert.True(t, *a.DestructiveHint)
		assert.False(t, *a.IdempotentHint)
	})

	t.Run("the spec can override the method", func(t *testing.T) {
		a := mcpAnnotationsForOperation(client, "searchRates")
		assert.True(t, *a.ReadOnlyHint)
		assert.False(t, *a.DestructiveHint)
	})

	t.Run("an unknown operation falls back to the cautious defaults", func(t *testing.T) {
		a := mcpAnnotationsForOperation(client, "noSuchOperation")
		assert.False(t, *a.ReadOnlyHint)
		assert.True(t, *a.DestructiveHint)
	})

	t.Run("annotations reach the wire format", func(t *testing.T) {
		tool := mcp.NewTool("getExchangeRate",
			mcp.WithToolAnnotation(mcpAnnotationsForOperation(client, "getExchangeRate")))

		encoded, err := json.Marshal(tool)
		require.NoError(t, err)

		var decoded struct {
			Annotations struct {
				ReadOnlyHint    *bool `json:"readOnlyHint"`
				DestructiveHint *bool `json:"destructiveHint"`
				IdempotentHint  *bool `json:"idempotentHint"`
				OpenWorldHint   *bool `json:"openWorldHint"`
			} `json:"annotations"`
		}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.NotNil(t, decoded.Annotations.DestructiveHint)
		assert.False(t, *decoded.Annotations.DestructiveHint)
		assert.True(t, *decoded.Annotations.ReadOnlyHint)
		assert.True(t, *decoded.Annotations.IdempotentHint)
		assert.True(t, *decoded.Annotations.OpenWorldHint)
	})
}
