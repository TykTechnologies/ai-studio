// A tiny Streamable HTTP MCP server for the live Tyk Gateway verification.
// Serves at /mcp on the given port and logs every request's auth header.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	port := "4020"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	s := server.NewMCPServer("weather-demo", "1.0.0", server.WithToolCapabilities(false))
	s.AddTool(mcp.NewTool("get-weather", mcp.WithDescription("Current weather for a city"), mcp.WithString("city", mcp.Required())),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			city, _ := req.Params.Arguments.(map[string]interface{})["city"].(string)
			return mcp.NewToolResultText(fmt.Sprintf("Sunny in %s, 21C", city)), nil
		})
	s.AddTool(mcp.NewTool("get-forecast", mcp.WithDescription("Three-day forecast"), mcp.WithString("city", mcp.Required())),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("Sunny, sunny, rain"), nil
		})
	h := server.NewStreamableHTTPServer(s, server.WithEndpointPath("/mcp"))
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s upstream-auth=%q x-upstream-token=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Upstream-Token"))
		h.ServeHTTP(w, r)
	}))
	log.Printf("mcp demo server on :%s/mcp", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
