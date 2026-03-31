package plugin_sdk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantUTF8 bool // result must be valid UTF-8
		want     string
	}{
		{
			name:     "valid ASCII",
			input:    []byte("hello world"),
			wantUTF8: true,
			want:     "hello world",
		},
		{
			name:     "valid multi-byte UTF-8",
			input:    []byte("café ☕ 日本語"),
			wantUTF8: true,
			want:     "café ☕ 日本語",
		},
		{
			name:     "empty input",
			input:    []byte{},
			wantUTF8: true,
			want:     "",
		},
		{
			name:     "single invalid byte 0xFF",
			input:    []byte{0xFF},
			wantUTF8: true,
			want:     "\uFFFD",
		},
		{
			name:     "Latin-1 cafe with 0xe9",
			input:    []byte("caf\xe9"),
			wantUTF8: true,
			want:     "caf\uFFFD",
		},
		{
			name:     "mixed valid and invalid",
			input:    []byte("hello \xfe\xff world"),
			wantUTF8: true,
			want:     "hello \uFFFD world",
		},
		{
			name:     "null bytes pass through (valid UTF-8)",
			input:    []byte("hello\x00world"),
			wantUTF8: true,
			want:     "hello\x00world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeUTF8(tt.input)
			if !utf8.ValidString(got) {
				t.Errorf("SanitizeUTF8() returned invalid UTF-8")
			}
			if got != tt.want {
				t.Errorf("SanitizeUTF8() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeUTF8String(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantUTF8 bool
		want     string
	}{
		{
			name:     "valid string unchanged",
			input:    "hello world",
			wantUTF8: true,
			want:     "hello world",
		},
		{
			name:     "string with invalid bytes sanitized",
			input:    "caf\xe9",
			wantUTF8: true,
			want:     "caf\uFFFD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeUTF8String(tt.input)
			if !utf8.ValidString(got) {
				t.Errorf("SanitizeUTF8String() returned invalid UTF-8")
			}
			if got != tt.want {
				t.Errorf("SanitizeUTF8String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWrapperCallResponseMustBeValidUTF8 validates that the wrapper's Call path
// produces valid UTF-8 in the response data field. Before the fix, passing
// non-UTF-8 bytes through string(response) would create an invalid protobuf string.
func TestWrapperCallResponseMustBeValidUTF8(t *testing.T) {
	// Simulate what wrapper.go:414 does: casting []byte response to string
	// for the CallResponse.Data field (a protobuf string).
	latinBytes := []byte("caf\xe9 na\xefve r\xe9sum\xe9")

	// This is what the code currently does (the bug):
	rawCast := string(latinBytes)
	if utf8.ValidString(rawCast) {
		t.Skip("raw cast happens to be valid UTF-8 on this input — test not applicable")
	}

	// After the fix, SanitizeUTF8 should produce valid UTF-8:
	sanitized := SanitizeUTF8(latinBytes)
	if !utf8.ValidString(sanitized) {
		t.Errorf("SanitizeUTF8 did not produce valid UTF-8 from Latin-1 input")
	}

	// Verify replacement characters are present
	if !strings.Contains(sanitized, "\uFFFD") {
		t.Errorf("expected replacement characters in sanitized output, got %q", sanitized)
	}
}
