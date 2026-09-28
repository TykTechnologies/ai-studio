package mcpintrospect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noop(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

// upstream serves an MCP server at /mcp that pages tools two at a time and,
// when token is set, requires "Authorization: Bearer <token>".
func upstream(t *testing.T, token string, tools ...mcp.Tool) *httptest.Server {
	t.Helper()
	s := server.NewMCPServer("orders", "1.2.3", server.WithToolCapabilities(false), server.WithPaginationLimit(2))
	for _, tool := range tools {
		s.AddTool(tool, noop)
	}
	h := server.NewStreamableHTTPServer(s)
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "missing or bad token", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListTools_PagesSortsAndDescribes(t *testing.T) {
	srv := upstream(t, "",
		mcp.NewTool("list_orders", mcp.WithDescription("List orders")),
		mcp.NewTool("get_order", mcp.WithDescription("Fetch one order"), mcp.WithTitleAnnotation("Get order")),
		mcp.NewTool("cancel_order"),
		mcp.NewTool("refund_order"),
		mcp.NewTool("ship_order"),
	)

	res, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: srv.Client(), ClientName: "test"})
	require.NoError(t, err)

	assert.Equal(t, "orders", res.ServerName)
	assert.Equal(t, "1.2.3", res.ServerVersion)
	assert.False(t, res.Truncated)
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	assert.Equal(t, []string{"cancel_order", "get_order", "list_orders", "refund_order", "ship_order"}, names, "every page is read and the list is sorted")
	assert.Equal(t, Tool{Name: "get_order", Title: "Get order", Description: "Fetch one order"}, res.Tools[1])
}

func TestListTools_SendsHeaders(t *testing.T) {
	srv := upstream(t, "s3cret", mcp.NewTool("ping"))

	_, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: srv.Client()})
	require.Error(t, err, "no token")
	assert.Contains(t, Describe(err), "401")

	res, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: srv.Client(), Headers: map[string]string{"Authorization": "Bearer s3cret"}})
	require.NoError(t, err)
	require.Len(t, res.Tools, 1)
	assert.Equal(t, "ping", res.Tools[0].Name)
}

func TestListTools_MaxTools(t *testing.T) {
	var tools []mcp.Tool
	for i := 0; i < 7; i++ {
		tools = append(tools, mcp.NewTool(fmt.Sprintf("tool_%d", i)))
	}
	srv := upstream(t, "", tools...)

	res, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: srv.Client(), MaxTools: 3})
	require.NoError(t, err)
	assert.Len(t, res.Tools, 3)
	assert.True(t, res.Truncated)
}

func TestListTools_ResponseCap(t *testing.T) {
	srv := upstream(t, "", mcp.NewTool("big", mcp.WithDescription(strings.Repeat("x", 4096))))

	_, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: srv.Client(), MaxResponseBytes: 1024})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrResponseTooLarge), "got %v", err)
}

func TestListTools_NotAnMCPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>" + strings.Repeat("hello ", 200) + "</html>"))
	}))
	t.Cleanup(srv.Close)

	_, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: srv.Client()})
	require.Error(t, err)
	assert.LessOrEqual(t, len([]rune(Describe(err))), maxErrorRunes+1, "the description is bounded")
}

func TestListTools_PolicyDialErrorSurvivesWrapping(t *testing.T) {
	srv := upstream(t, "", mcp.NewTool("ping"))
	blocked := netguard.NewURLPolicy(netguard.PolicyOptions{}).NewHTTPClient(5 * time.Second)

	_, err := ListTools(context.Background(), srv.URL+"/mcp", Options{HTTPClient: blocked})
	require.Error(t, err)
	assert.True(t, errors.Is(err, netguard.ErrPolicyDial), "callers map this sentinel; got %v", err)
}

func TestListTools_RequiresClient(t *testing.T) {
	_, err := ListTools(context.Background(), "http://example.invalid/mcp", Options{})
	assert.Error(t, err)
}
