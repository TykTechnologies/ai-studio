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
	s = strings.TrimSpace(s)
	// A data URI is one token with a type/subtype (or no) media type. An
	// SSE body ("data: {...}\n\n") also starts with "data:" and holds a
	// comma; it is text, never a blob (jyx.146).
	if len(s) > 5 && strings.EqualFold(s[:5], "data:") && !strings.ContainsAny(s, " \t\r\n") {
		rest := s[5:]
		meta, _, hasData := strings.Cut(rest, ",")
		if hasData {
			mt, _, _ := strings.Cut(meta, ";")
			if mt == "" {
				mt = "text/plain"
			}
			if mediaTypeToken(mt) {
				return strings.ToLower(mt), true
			}
		}
	}
	if len(s) > MinBase64Bytes && isBase64(s) {
		return "application/octet-stream", true
	}
	return "", false
}

// mediaTypeToken reports whether mt is type/subtype made of RFC 2045 token
// characters.
func mediaTypeToken(mt string) bool {
	typ, sub, ok := strings.Cut(mt, "/")
	if !ok || typ == "" || sub == "" || len(mt) > 127 {
		return false
	}
	for i := 0; i < len(mt); i++ {
		c := mt[i]
		if c == '/' && i == len(typ) {
			continue
		}
		if c <= ' ' || c >= 0x7f || strings.IndexByte(`()<>@,;:\"/[]?={}`, c) >= 0 {
			return false
		}
	}
	return true
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

// minBase64Run is the shortest whitespace-separated run of wrapped base64
// (MIME wraps at 76): prose made only of letters and spaces has shorter
// words, so it is never taken for binary.
const minBase64Run = 40

// isBase64 accepts the standard and URL-safe alphabets and padding, wrapped
// by whitespace into runs of at least minBase64Run (the last may be
// shorter), and nothing else.
func isBase64(s string) bool {
	run := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			run++
		case c == '+', c == '/', c == '-', c == '_', c == '=':
			run++
		case c == '\n', c == '\r', c == ' ', c == '\t':
			if run > 0 && run < minBase64Run {
				return false
			}
			run = 0
		default:
			return false
		}
	}
	return true
}
