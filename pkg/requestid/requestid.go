// Package requestid gives every gateway request one id of AI Studio's own.
//
// The id is minted once, at the gateway's entry (Studio's proxy handler, the
// Microgateway's first middleware), and travels in the request context under
// the "request_id" key that plugins, hooks, filters and analytics already
// read. It is returned to the client in Header, never in X-Request-ID, which
// belongs to the client and to whatever proxies sit in front of the gateway:
// their X-Request-ID is passed through untouched and recorded as the client
// request id.
//
// Ids look like "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b": a fixed prefix,
// then a UUIDv7 in hex. UUIDv7 is ordered by time and carries 74 random bits,
// so ids minted on different nodes (edges, Studio replicas) do not collide;
// the prefix tells them apart from other systems' ids in shared logs.
package requestid

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const (
	// Header carries the gateway's request id on every response, and on the
	// gateway's own loopback hop, where it is trusted only from that hop.
	Header = "X-Tyk-AI-Request-ID"
	// ClientHeader is the caller's own request id, recorded, never set.
	ClientHeader = "X-Request-ID"
	// Prefix starts every id this package mints.
	Prefix = "tyai_"
	// ContextKey is the context key the id is stored under. It is a plain
	// string because plugins and hooks have always read it that way.
	ContextKey = "request_id"
	// ClientContextKey holds the caller's sanitised X-Request-ID.
	ClientContextKey = "client_request_id"
	// MaxClientLen bounds a recorded client request id.
	MaxClientLen = 128
)

// New mints an id.
func New() string {
	u, err := uuid.NewV7()
	if err != nil {
		u = uuid.New() // the random source failed for v7; v4 is still unique
	}
	return Prefix + strings.ReplaceAll(u.String(), "-", "")
}

// Valid reports whether id has the shape New gives, so a header value can be
// trusted to be one of ours and not text chosen to write into logs.
func Valid(id string) bool {
	if len(id) != len(Prefix)+32 || !strings.HasPrefix(id, Prefix) {
		return false
	}
	for _, c := range id[len(Prefix):] {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// WithID stores id (and the caller's id, when not empty) in ctx.
func WithID(ctx context.Context, id, clientID string) context.Context {
	ctx = context.WithValue(ctx, ContextKey, id) //nolint:staticcheck // plugins read the plain string key
	if clientID != "" {
		ctx = context.WithValue(ctx, ClientContextKey, clientID) //nolint:staticcheck // as above
	}
	return ctx
}

// FromContext returns the request id in ctx, "" when there is none.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ContextKey).(string)
	return id
}

// ClientFromContext returns the caller's request id in ctx, "" when none.
func ClientFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ClientContextKey).(string)
	return id
}

// SanitizeClient keeps a caller-supplied id to MaxClientLen characters of
// letters, digits and - _ . : / so it cannot write into a log line or event.
func SanitizeClient(id string) string {
	var b strings.Builder
	for _, r := range id {
		if b.Len() >= MaxClientLen {
			break
		}
		if r == '-' || r == '_' || r == '.' || r == ':' || r == '/' ||
			('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
