// Package mcpintrospect asks a remote MCP server which tools it offers. It
// speaks Streamable HTTP through a caller-supplied http.Client, so the caller
// decides where the request may go (its outbound URL policy) and this package
// only adds a response-size cap.
package mcpintrospect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultMaxTools         = 1000
	defaultMaxResponseBytes = 8 << 20
	maxPages                = 50
	maxDescriptionRunes     = 2000
	maxErrorRunes           = 300
)

// ErrResponseTooLarge is returned when one response exceeds MaxResponseBytes.
var ErrResponseTooLarge = errors.New("the MCP server's response is too large")

// Tool is one tool the server advertises in tools/list.
type Tool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// Result is what the server reported.
type Result struct {
	ServerName    string `json:"server_name,omitempty"`
	ServerVersion string `json:"server_version,omitempty"`
	Tools         []Tool `json:"tools"`
	// Truncated is set when the server offered more tools than MaxTools.
	Truncated bool `json:"truncated,omitempty"`
}

// Options configures one introspection.
type Options struct {
	// HTTPClient carries the caller's outbound policy. Required.
	HTTPClient *http.Client
	// Headers are sent on every request (for example the upstream auth header).
	Headers map[string]string
	// MaxTools caps the list; defaults to 1000.
	MaxTools int
	// MaxResponseBytes caps each response body; defaults to 8 MB.
	MaxResponseBytes int64
	// ClientName and ClientVersion identify the caller in initialize.
	ClientName    string
	ClientVersion string
}

// ListTools initializes a session with the MCP endpoint, pages through
// tools/list and closes the session. Tools are sorted by name and
// de-duplicated.
func ListTools(ctx context.Context, endpoint string, o Options) (*Result, error) {
	if o.HTTPClient == nil {
		return nil, errors.New("mcpintrospect: an HTTP client is required")
	}
	if o.MaxTools <= 0 {
		o.MaxTools = defaultMaxTools
	}
	if o.MaxResponseBytes <= 0 {
		o.MaxResponseBytes = defaultMaxResponseBytes
	}
	if o.ClientName == "" {
		o.ClientName = "mcpintrospect"
	}

	hc := *o.HTTPClient
	base := hc.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	hc.Transport = capTransport{base: base, max: o.MaxResponseBytes}

	opts := []transport.StreamableHTTPCOption{
		transport.WithHTTPBasicClient(&hc),
		transport.WithHTTPLogger(quietLogger{}),
	}
	if len(o.Headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(o.Headers))
	}
	c, err := mcpclient.NewStreamableHttpClient(endpoint, opts...)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		return nil, err
	}

	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: o.ClientName, Version: o.ClientVersion}
	info, err := c.Initialize(ctx, init)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	res := &Result{ServerName: info.ServerInfo.Name, ServerVersion: info.ServerInfo.Version, Tools: []Tool{}}
	seen := map[string]bool{}
	var cursor mcp.Cursor
	for page := 0; page < maxPages; page++ {
		req := mcp.ListToolsRequest{}
		req.Params.Cursor = cursor
		out, err := c.ListToolsByPage(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		for _, t := range out.Tools {
			if t.Name == "" || seen[t.Name] {
				continue
			}
			if len(res.Tools) == o.MaxTools {
				res.Truncated = true
				break
			}
			seen[t.Name] = true
			res.Tools = append(res.Tools, Tool{Name: t.Name, Title: t.Annotations.Title, Description: truncate(t.Description, maxDescriptionRunes)})
		}
		if res.Truncated || out.NextCursor == "" || out.NextCursor == cursor {
			break
		}
		cursor = out.NextCursor
	}
	sort.Slice(res.Tools, func(i, j int) bool { return res.Tools[i].Name < res.Tools[j].Name })
	return res, nil
}

// Describe shortens an introspection error for display: the MCP client
// includes the server's response body in its errors.
func Describe(err error) string {
	if err == nil {
		return ""
	}
	return truncate(strings.Join(strings.Fields(err.Error()), " "), maxErrorRunes)
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// capTransport limits every response body to max bytes.
type capTransport struct {
	base http.RoundTripper
	max  int64
}

func (t capTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.Body == nil {
		return resp, err
	}
	resp.Body = &cappedBody{rc: resp.Body, left: t.max}
	return resp, nil
}

type cappedBody struct {
	rc   io.ReadCloser
	left int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	if b.left <= 0 {
		// One byte past the cap tells a body that ends exactly at the cap
		// from one that goes on.
		var one [1]byte
		if n, _ := b.rc.Read(one[:]); n > 0 {
			return 0, ErrResponseTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.rc.Read(p)
	b.left -= int64(n)
	return n, err
}

func (b *cappedBody) Close() error { return b.rc.Close() }

type quietLogger struct{}

func (quietLogger) Infof(string, ...any)  {}
func (quietLogger) Errorf(string, ...any) {}
