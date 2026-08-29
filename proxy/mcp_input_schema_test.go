package proxy

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exchangeRatesSpec mirrors the shape reported in #484: two required path
// parameters and one optional query parameter, each described on the parameter
// object as authors normally write them.
const exchangeRatesSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "Currency Exchange Rates", "version": "1.0.0"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/rate/{base}/{quote}": {
      "get": {
        "operationId": "getExchangeRate",
        "description": "Look up the exchange rate between two currencies.",
        "parameters": [
          {
            "name": "base",
            "in": "path",
            "required": true,
            "description": "ISO 4217 code of the currency being converted from.",
            "schema": {"type": "string"}
          },
          {
            "name": "quote",
            "in": "path",
            "required": true,
            "description": "ISO 4217 code of the currency being converted to.",
            "schema": {"type": "string"}
          },
          {
            "name": "date",
            "in": "query",
            "required": false,
            "description": "Rate date in YYYY-MM-DD; defaults to the latest.",
            "schema": {"type": "string"}
          }
        ],
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

// buildMCPTool runs a spec through the same path the MCP bridge uses:
// AsTool -> mcpParameterOptions -> mcp.NewTool.
func buildMCPTool(t *testing.T, spec, operationID string) mcp.Tool {
	t.Helper()

	client, err := universalclient.NewClient([]byte(spec), "")
	require.NoError(t, err)

	tools, err := client.AsTool(operationID)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	opts := []mcp.ToolOption{mcp.WithDescription(tools[0].Function.Description)}
	opts = append(opts, mcpParameterOptions(tools[0].Function.Parameters)...)
	return mcp.NewTool(tools[0].Function.Name, opts...)
}

func TestMCPInputSchemaMarksRequiredParameters(t *testing.T) {
	tool := buildMCPTool(t, exchangeRatesSpec, "getExchangeRate")

	assert.ElementsMatch(t, []string{"base", "quote"}, tool.InputSchema.Required,
		"required path parameters must surface as required over MCP")

	// And through the wire format an MCP client actually reads.
	encoded, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)

	var inputSchema struct {
		Required   []string                          `json:"required"`
		Properties map[string]map[string]interface{} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(encoded, &inputSchema))
	assert.ElementsMatch(t, []string{"base", "quote"}, inputSchema.Required)
	assert.Contains(t, inputSchema.Properties, "date")
}

func TestMCPInputSchemaCarriesParameterDescriptions(t *testing.T) {
	tool := buildMCPTool(t, exchangeRatesSpec, "getExchangeRate")

	for name, want := range map[string]string{
		"base":  "ISO 4217 code of the currency being converted from.",
		"quote": "ISO 4217 code of the currency being converted to.",
		"date":  "Rate date in YYYY-MM-DD; defaults to the latest.",
	} {
		prop, ok := tool.InputSchema.Properties[name].(map[string]interface{})
		require.Truef(t, ok, "missing property %q", name)
		assert.Equalf(t, want, prop["description"],
			"parameter %q lost the description written on the OpenAPI parameter object", name)
	}
}

func TestMCPInputSchemaOmitsEmptyDescriptions(t *testing.T) {
	const spec = `{
      "openapi": "3.0.0",
      "info": {"title": "Bare", "version": "1.0.0"},
      "servers": [{"url": "https://api.example.com"}],
      "paths": {
        "/thing": {
          "get": {
            "operationId": "getThing",
            "parameters": [{"name": "id", "in": "query", "schema": {"type": "string"}}],
            "responses": {"200": {"description": "OK"}}
          }
        }
      }
    }`

	tool := buildMCPTool(t, spec, "getThing")
	prop, ok := tool.InputSchema.Properties["id"].(map[string]interface{})
	require.True(t, ok)
	_, hasDescription := prop["description"]
	assert.False(t, hasDescription, `a parameter with no description must not publish "description": ""`)
}

func TestSchemaRequiredNames(t *testing.T) {
	// buildParametersSchema emits []string; a JSON round trip yields
	// []interface{}. Both have to be understood.
	assert.Equal(t, []string{"base", "quote"}, schemaRequiredNames([]string{"base", "quote"}))
	assert.Equal(t, []string{"base", "quote"}, schemaRequiredNames([]interface{}{"base", "quote"}))
	assert.Equal(t, []string{"base"}, schemaRequiredNames([]interface{}{"base", 42, nil}))
	assert.Nil(t, schemaRequiredNames(nil))
	assert.Nil(t, schemaRequiredNames("base"))
}

func TestFlattenSchemaPropertiesKeepsNestedRequired(t *testing.T) {
	// SchemaToMap emits a nested object's "required" as []string too.
	properties := map[string]interface{}{
		"body": map[string]interface{}{
			"type":     "object",
			"required": []string{"amount"},
			"properties": map[string]interface{}{
				"amount": map[string]interface{}{"type": "number", "description": "Amount to convert."},
				"memo":   map[string]interface{}{"type": "string"},
			},
		},
	}

	flattened, required := flattenSchemaProperties(properties, []string{"body"}, "")

	assert.Contains(t, flattened, "body_amount")
	assert.Contains(t, flattened, "body_memo")
	assert.Equal(t, []string{"body_amount"}, required)
	assert.Equal(t, "Amount to convert.", flattened["body_amount"]["description"])
}
