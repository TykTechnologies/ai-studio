package agenttrace

// contentAttributes are span attributes that carry captured content. One
// list for every reader: the JSON v1 stripped form drops them, the OTel
// exporter treats them as content, and the hub drops them from an edge
// event's attributes at ingest, since content travels only in payload
// parts (sealed at rest, D4), never in the attributes column (jyx.161).
var contentAttributes = map[string]bool{
	"gen_ai.input.messages":       true,
	"gen_ai.output.messages":      true,
	"gen_ai.system_instructions":  true,
	"gen_ai.tool.definitions":     true,
	"gen_ai.tool.call.arguments":  true,
	"gen_ai.tool.call.result":     true,
	"gen_ai.prompt":               true,
	"gen_ai.completion":           true,
	"tyk.error.body":              true,
	"mcp.request.params":          true,
	"mcp.response.result":         true,
	"http.request.body.content":   true,
	"http.response.body.content":  true,
	"tyk.datasource.query":        true,
	"tyk.datasource.result":       true,
	"tyk.plugin.request.content":  true,
	"tyk.plugin.response.content": true,
}

// IsContentAttribute reports whether a span attribute carries captured
// content.
func IsContentAttribute(name string) bool { return contentAttributes[name] }
