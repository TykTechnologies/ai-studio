package plugin_sdk

import (
	"strings"
	"unicode/utf8"
)

// SanitizeUTF8 ensures a byte slice produces a valid UTF-8 string by replacing
// invalid sequences with the Unicode replacement character (U+FFFD).
// This is necessary because protobuf3 string fields require valid UTF-8.
func SanitizeUTF8(b []byte) string {
	s := string(b)
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// SanitizeUTF8String ensures a string is valid UTF-8 by replacing invalid
// sequences with the Unicode replacement character (U+FFFD).
func SanitizeUTF8String(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}
