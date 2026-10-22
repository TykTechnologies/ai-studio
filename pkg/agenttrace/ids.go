package agenttrace

import (
	"crypto/rand"
	"encoding/hex"
)

// TraceID is a W3C trace id (16 bytes). The zero value means unset.
type TraceID [16]byte

// SpanID is a W3C span id (8 bytes). The zero value means unset.
type SpanID [8]byte

// IsZero reports whether the id is unset.
func (t TraceID) IsZero() bool { return t == TraceID{} }

// IsZero reports whether the id is unset.
func (s SpanID) IsZero() bool { return s == SpanID{} }

// String renders the id as lowercase hex (W3C format), or "" when unset.
func (t TraceID) String() string {
	if t.IsZero() {
		return ""
	}
	return hex.EncodeToString(t[:])
}

// String renders the id as lowercase hex (W3C format), or "" when unset.
func (s SpanID) String() string {
	if s.IsZero() {
		return ""
	}
	return hex.EncodeToString(s[:])
}

// Bytes returns the id as a slice, or nil when unset (the column is NULL).
func (t TraceID) Bytes() []byte {
	if t.IsZero() {
		return nil
	}
	return append([]byte(nil), t[:]...)
}

// Bytes returns the id as a slice, or nil when unset (the column is NULL).
func (s SpanID) Bytes() []byte {
	if s.IsZero() {
		return nil
	}
	return append([]byte(nil), s[:]...)
}

// NewTraceID returns a random non-zero trace id from crypto/rand.
func NewTraceID() TraceID {
	var t TraceID
	for t.IsZero() {
		_, _ = rand.Read(t[:])
	}
	return t
}

// NewSpanID returns a random non-zero span id from crypto/rand.
func NewSpanID() SpanID {
	var s SpanID
	for s.IsZero() {
		_, _ = rand.Read(s[:])
	}
	return s
}

// ParseTraceID parses 32 hex characters. It returns false for anything else,
// including the all-zero id, which W3C defines as invalid.
func ParseTraceID(s string) (TraceID, bool) {
	var t TraceID
	if len(s) != 32 {
		return t, false
	}
	if _, err := hex.Decode(t[:], []byte(s)); err != nil {
		return TraceID{}, false
	}
	return t, !t.IsZero()
}

// ParseSpanID parses 16 hex characters. It returns false for anything else,
// including the all-zero id.
func ParseSpanID(s string) (SpanID, bool) {
	var id SpanID
	if len(s) != 16 {
		return id, false
	}
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		return SpanID{}, false
	}
	return id, !id.IsZero()
}

// traceIDFrom copies b into a TraceID; wrong lengths give the zero id.
func traceIDFrom(b []byte) TraceID {
	var t TraceID
	if len(b) == len(t) {
		copy(t[:], b)
	}
	return t
}

// spanIDFrom copies b into a SpanID; wrong lengths give the zero id.
func spanIDFrom(b []byte) SpanID {
	var s SpanID
	if len(b) == len(s) {
		copy(s[:], b)
	}
	return s
}
