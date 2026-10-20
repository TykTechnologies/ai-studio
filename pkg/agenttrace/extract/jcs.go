package extract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// maxDepth bounds JSON nesting in Canonicalize, so a hostile body cannot
// exhaust the stack.
const maxDepth = 128

var (
	// ErrDuplicateKey: an object repeats a key. RFC 8785 requires unique
	// keys, and two parsers could read such a body differently.
	ErrDuplicateKey = errors.New("extract: duplicate object key")
	// ErrTooDeep: the JSON nests deeper than maxDepth.
	ErrTooDeep = errors.New("extract: JSON nested too deeply")
	// ErrNumber: a number is outside the IEEE 754 double range.
	ErrNumber = errors.New("extract: number out of range")
)

// value is a parsed JSON value: nil, bool, json.Number, string, []value or
// object.
type value = any

// object keeps members in input order; serialisation sorts them.
type object []member

type member struct {
	key string
	val value
}

// Canonicalize returns the RFC 8785 (JCS) form of a JSON text: members
// sorted by UTF-16 code units, no whitespace, numbers in ECMAScript form,
// strings with minimal escaping. Duplicate keys are an error.
func Canonicalize(data []byte) ([]byte, error) {
	v, err := parse(data)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Grow(len(data))
	if err := write(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parse(data []byte) (value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("extract: trailing data after JSON value")
	}
	return v, nil
}

func parseValue(dec *json.Decoder, depth int) (value, error) {
	if depth > maxDepth {
		return nil, ErrTooDeep
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			var obj object
			seen := make(map[string]struct{})
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				if _, dup := seen[k]; dup {
					return nil, ErrDuplicateKey
				}
				seen[k] = struct{}{}
				v, err := parseValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				obj = append(obj, member{key: k, val: v})
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			if obj == nil {
				obj = object{}
			}
			return obj, nil
		case '[':
			arr := []value{}
			for dec.More() {
				v, err := parseValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("extract: unexpected delimiter %q", t)
	default:
		return t, nil
	}
}

func write(buf *bytes.Buffer, v value) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		s, err := formatNumber(string(t))
		if err != nil {
			return err
		}
		buf.WriteString(s)
	case string:
		writeString(buf, t)
	case []value:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := write(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case object:
		sorted := make(object, len(t))
		copy(sorted, t)
		sort.Slice(sorted, func(i, j int) bool { return lessUTF16(sorted[i].key, sorted[j].key) })
		buf.WriteByte('{')
		for i, m := range sorted {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, m.key)
			buf.WriteByte(':')
			if err := write(buf, m.val); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("extract: unexpected value %T", v)
	}
	return nil
}

// lessUTF16 orders keys by their UTF-16 code units (RFC 8785 §3.2.3).
func lessUTF16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

const hexDigits = "0123456789abcdef"

// writeString escapes per RFC 8785 §3.2.2.2: only '"', '\\' and control
// characters, with the short forms where JSON has them.
func writeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if c < 0x20 {
				buf.WriteString(`\u00`)
				buf.WriteByte(hexDigits[c>>4])
				buf.WriteByte(hexDigits[c&0xf])
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
}

// formatNumber renders a JSON number as ECMAScript Number.prototype.toString
// does for its IEEE 754 double value (RFC 8785 §3.2.2.3).
func formatNumber(lit string) (string, error) {
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", ErrNumber
	}
	if f == 0 {
		return "0", nil // also -0
	}
	var sb strings.Builder
	if f < 0 {
		sb.WriteByte('-')
		f = -f
	}
	// Shortest round-trip digits: "d.ddddde±XX".
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	k := len(digits)
	n := x + 1 // value = 0.digits × 10^n
	switch {
	case k <= n && n <= 21:
		sb.WriteString(digits)
		sb.WriteString(strings.Repeat("0", n-k))
	case 0 < n && n <= 21:
		sb.WriteString(digits[:n])
		sb.WriteByte('.')
		sb.WriteString(digits[n:])
	case -6 < n && n <= 0:
		sb.WriteString("0.")
		sb.WriteString(strings.Repeat("0", -n))
		sb.WriteString(digits)
	default:
		sb.WriteByte(digits[0])
		if k > 1 {
			sb.WriteByte('.')
			sb.WriteString(digits[1:])
		}
		sb.WriteByte('e')
		if n-1 >= 0 {
			sb.WriteByte('+')
		} else {
			sb.WriteByte('-')
		}
		sb.WriteString(strconv.Itoa(abs(n - 1)))
	}
	return sb.String(), nil
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
