package mcpserver

import (
	"context"
	"net/http"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/gin-gonic/gin"
)

// HTTPHandler is an HTTP handler for MCP server
type HTTPHandler struct {
	server  *mcp.Server
	sseServ *mcp.SSEServer
}

// NewHTTPHandler creates a new HTTP handler for MCP server
func NewHTTPHandler(server *mcp.Server, sseServer *mcp.SSEServer) *HTTPHandler {
	return &HTTPHandler{
		server:  server,
		sseServ: sseServer,
	}
}

// ServeHTTP handles HTTP requests
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check for SSE requests
	if r.URL.Path == "/sse" {
		h.sseServ.HandleSSE().ServeHTTP(w, r)
		return
	}

	// Check for message requests
	if r.URL.Path == "/message" {
		h.sseServ.HandleMessage().ServeHTTP(w, r)
		return
	}

	// Default handler
	http.NotFound(w, r)
}

// RegisterRoutes registers the MCP server routes with a Gin engine
func (h *HTTPHandler) RegisterRoutes(router *gin.Engine, basePath string) {
	// Register SSE endpoint
	router.GET(basePath+"/sse", func(c *gin.Context) {
		h.sseServ.HandleSSE().ServeHTTP(c.Writer, c.Request)
	})
	// Register message endpoint
	router.POST(basePath+"/message", func(c *gin.Context) {
		h.sseServ.HandleMessage().ServeHTTP(c.Writer, c.Request)
	})
}

// Start starts the MCP server
func (h *HTTPHandler) Start() {
	// Start the MCP server (non-blocking)
	go func() {
		h.server.Serve()
	}()
}

// Shutdown gracefully shuts down the MCP server
func (h *HTTPHandler) Shutdown(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}
