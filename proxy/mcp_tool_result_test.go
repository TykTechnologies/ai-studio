package proxy

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textOf pulls the single text block out of an MCP tool result.
func textOf(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected a text content block")
	return text.Text
}

func TestMCPToolResultFromValue(t *testing.T) {
	t.Run("a map is rendered as JSON, not Go map syntax", func(t *testing.T) {
		// universalclient returns a map when the operation documents a response
		// schema for the status code. This used to come back as
		// "map[base:EUR date:2026-08-28 quote:GBP rate:0.85765]".
		result := mcpToolResultFromValue("getExchangeRate", map[string]interface{}{
			"date":  "2026-08-28",
			"base":  "EUR",
			"quote": "GBP",
			"rate":  0.85765,
		})

		text := textOf(t, result)
		assert.NotContains(t, text, "map[")

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(text), &decoded), "result must be valid JSON")
		assert.Equal(t, "EUR", decoded["base"])
		assert.Equal(t, "GBP", decoded["quote"])
		assert.Equal(t, "2026-08-28", decoded["date"])
		assert.InDelta(t, 0.85765, decoded["rate"], 1e-9)
	})

	t.Run("map key order is stable across calls", func(t *testing.T) {
		payload := map[string]interface{}{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
		first := textOf(t, mcpToolResultFromValue("op", payload))
		for i := 0; i < 20; i++ {
			assert.Equal(t, first, textOf(t, mcpToolResultFromValue("op", payload)))
		}
	})

	t.Run("a slice is rendered as a JSON array", func(t *testing.T) {
		result := mcpToolResultFromValue("listRates", []interface{}{
			map[string]interface{}{"quote": "GBP"},
			map[string]interface{}{"quote": "USD"},
		})

		text := textOf(t, result)
		var decoded []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(text), &decoded))
		require.Len(t, decoded, 2)
		assert.Equal(t, "GBP", decoded[0]["quote"])
	})

	t.Run("a string passes through untouched", func(t *testing.T) {
		// An operation with no documented response schema already came back as
		// the raw upstream body; re-encoding it would double-quote the JSON.
		const body = `{"date":"2026-08-28","rate":0.85765}`
		assert.Equal(t, body, textOf(t, mcpToolResultFromValue("getExchangeRate", body)))
	})

	t.Run("a nil result is rendered as JSON null", func(t *testing.T) {
		assert.Equal(t, "null", textOf(t, mcpToolResultFromValue("op", nil)))
	})

	t.Run("an unmarshalable value falls back to Go formatting", func(t *testing.T) {
		// json.Marshal rejects NaN. The call must still return content rather
		// than dropping the tool result.
		text := textOf(t, mcpToolResultFromValue("op", map[string]interface{}{"x": math.NaN()}))
		assert.NotEmpty(t, text)
	})
}
