package extract

import "strings"

// MinBase64Bytes is the §3.4 threshold: base64 strings longer than this are
// treated as binary.
const MinBase64Bytes = 1024

// BinaryString reports whether s is binary content the content pipeline must
// replace with a $tyk_blob reference (§3.4 payload rules): a data URI, or a
// base64 run over MinBase64Bytes. mediaType is the data URI's type, or
// application/octet-stream.
func BinaryString(s string) (mediaType string, ok bool) {
	if rest, found := strings.CutPrefix(s, "data:"); found {
		meta, _, hasData := strings.Cut(rest, ",")
		if hasData {
			mt, _, _ := strings.Cut(meta, ";")
			if mt == "" {
				mt = "text/plain"
			}
			return strings.ToLower(mt), true
		}
	}
	if len(s) > MinBase64Bytes && isBase64(s) {
		return "application/octet-stream", true
	}
	return "", false
}

// BinaryMediaType reports whether a declared media type (an image part's
// media_type, an inlineData mimeType) is binary rather than text.
func BinaryMediaType(mt string) bool {
	mt = strings.ToLower(strings.TrimSpace(mt))
	if mt == "" {
		return false
	}
	if strings.HasPrefix(mt, "text/") {
		return false
	}
	switch mt {
	case "application/json", "application/xml", "application/x-ndjson", "application/yaml":
		return false
	}
	return true
}

// isBase64 accepts the standard and URL-safe alphabets, padding and line
// breaks, and nothing else.
func isBase64(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '+', c == '/', c == '-', c == '_', c == '=', c == '\n', c == '\r':
		default:
			return false
		}
	}
	return true
}
