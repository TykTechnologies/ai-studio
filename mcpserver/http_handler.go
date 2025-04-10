package mcpserver

import (
	"net/http"

	"github.com/ThinkInAIXYZ/go-mcp/server"
	"github.com/ThinkInAIXYZ/go-mcp/transport"
)

// HTTPHandler wraps MCP server handlers for HTTP routes
type HTTPHandler struct {
	mcpServer  *server.Server
	SSEHandler *transport.SSEHandler
}

// NewHTTPHandler creates a new HTTP handler for an MCP server
func NewHTTPHandler(mcpServer *server.Server, sseHandler *transport.SSEHandler) *HTTPHandler {
	return &HTTPHandler{
		mcpServer:  mcpServer,
		SSEHandler: sseHandler,
	}
}

// HandleSSE handles Server-Sent Events connections
func (h *HTTPHandler) HandleSSE() http.Handler {
	return h.SSEHandler.HandleSSE()
}

// HandleMessage handles JSON-RPC messages
func (h *HTTPHandler) HandleMessage() http.Handler {
	return h.SSEHandler.HandleMessage()
}
