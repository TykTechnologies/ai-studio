package segments

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// decodedView returns the strings of a JSON object or array held in text,
// keys and values, decoded and in order, one per line, for classification.
// Tool output is often JSON a tool serialised into a string: the model reads
// its \u escapes and \n as the characters they stand for, while a pattern
// over the raw text sees backslashes. Every copy of a repeated key is
// included, since the model reads them all. ok is false when text is not a
// JSON object or array.
func decodedView(text string) (view string, ok bool) {
	t := strings.TrimSpace(text)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(t))
	dec.UseNumber()
	var parts []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", false
		}
		if s, isString := tok.(string); isString && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n"), true
}

// JSONView is the text a tool result's JSON escapes stand for: decodedView
// when the text is a JSON document, else, when it holds escapes at all, the
// text with every JSON escape sequence decoded wherever it occurs, so JSON
// with prose around it, trailing text or a cut-off end is read too. ok is
// false when neither applies.
func JSONView(text string) (view string, ok bool) {
	if v, ok := decodedView(text); ok {
		return v, v != ""
	}
	if !strings.Contains(text, `\`) {
		return "", false
	}
	v := unescape(text)
	return v, v != text
}

// unescape decodes the JSON escape sequences in text (\uXXXX including
// surrogate pairs, \n, \r, \t, \b, \f, \", \\, \/) in one pass. Anything
// that is not a valid escape is kept as it is.
func unescape(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c != '\\' || i+1 >= len(text) {
			b.WriteByte(c)
			continue
		}
		switch e := text[i+1]; e {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case '"', '\\', '/':
			b.WriteByte(e)
		case 'u':
			r, n := unicodeEscape(text[i:])
			if n == 0 {
				b.WriteByte(c)
				continue
			}
			b.WriteRune(r)
			i += n - 1
			continue
		default:
			b.WriteByte(c)
			continue
		}
		i++
	}
	return b.String()
}

// unicodeEscape reads a \uXXXX escape at the start of s, joining a surrogate
// pair. n is the number of bytes read, 0 when s does not start with one.
func unicodeEscape(s string) (r rune, n int) {
	hi, ok := hex4(s)
	if !ok {
		return 0, 0
	}
	if utf16.IsSurrogate(hi) {
		if lo, ok := hex4(s[6:]); ok {
			if pair := utf16.DecodeRune(hi, lo); pair != utf8.RuneError {
				return pair, 12
			}
		}
		return utf8.RuneError, 6
	}
	return hi, 6
}

// hex4 reads `\uXXXX` at the start of s.
func hex4(s string) (rune, bool) {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return 0, false
	}
	v, err := strconv.ParseUint(s[2:6], 16, 32)
	if err != nil {
		return 0, false
	}
	return rune(v), true
}
