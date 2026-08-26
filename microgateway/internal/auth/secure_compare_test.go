package auth

import "testing"

// Issue #404: token comparisons performed in-process must use a
// constant-time comparison to avoid timing side channels.

func TestSecureTokenEquals(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"equal tokens", "abc123def456", "abc123def456", true},
		{"different tokens same length", "abc123def456", "abc123def457", false},
		{"different lengths", "short", "much-longer-token", false},
		{"empty vs empty", "", "", false}, // empty credentials never authenticate
		{"empty vs value", "", "token", false},
		{"value vs empty", "token", "", false},
		{"case sensitive", "AbC", "abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SecureTokenEquals(tc.a, tc.b); got != tc.want {
				t.Errorf("SecureTokenEquals(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
