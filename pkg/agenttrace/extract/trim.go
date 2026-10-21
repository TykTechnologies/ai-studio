package extract

import (
	"bytes"
	"strings"
)

// History trimming (jyx.57). A request resends the whole conversation, but
// only what follows the model's last message is extracted: the trailing
// tool results, and the last role. Segmenting every earlier message costs
// time and allocations in proportion to the history, so trimHistory cuts
// the conversation array to start at its last assistant message, with one
// pass over the bytes and no tree.
//
// The segmenter numbers turns by the assistant messages before a unit, so
// cutting at an assistant message shifts every kept unit's turn by the same
// amount and keeps their order. The result is the whole body's whenever the
// trimmed body holds a non-assistant unit in a message after the cut
// (parseUnits' settled); parseRequest falls back to the whole body
// otherwise, and when the trimmed body fails.
//
// The segmenter validates what is kept; the history before the cut is only
// scanned for its structure, so a history the segmenter would refuse (a
// repeated key or broken JSON in an old message) is not seen on this path.
// The vendor validates the body it serves. Any byte the scanner does not
// expect makes it give up, and the whole body is parsed as before.

// historyKey is the conversation array of a format and the role value of
// its assistant messages, as the segmenter reads them; "" when the format
// is not trimmed.
func historyKey(format string) (key, assistant string) {
	switch format {
	case FormatOpenAI, FormatOllama, FormatAnthropic, FormatBedrock:
		return "messages", "assistant"
	case FormatGoogleAI, FormatVertex:
		return "contents", "model"
	}
	return "", ""
}

// inHistory reports whether a unit's path lies in the conversation array.
func inHistory(format, path string) bool {
	key, _ := historyKey(format)
	return key != "" && strings.HasPrefix(path, "/"+key+"/")
}

// trimHistory returns body with its conversation array cut to start at the
// last assistant message, or false when there is nothing to cut or the body
// is not plainly shaped: not an object, the array key repeated or escaped,
// no assistant message after the first, or anything malformed. A message
// counts as the assistant's only when its one role key holds exactly the
// assistant value, so an escaped or repeated role is never a cut.
func trimHistory(format string, body []byte) ([]byte, bool) {
	key, assistant := historyKey(format)
	if key == "" {
		return nil, false
	}
	arr := -1
	ok := members(body, skipSpace(body, 0), func(k []byte, v, _ int) bool {
		if bytes.IndexByte(k, '\\') >= 0 {
			return false // an escaped key could be the array's
		}
		if string(k) == key {
			if arr >= 0 {
				return false
			}
			arr = v
		}
		return true
	})
	if !ok || arr < 0 || body[arr] != '[' {
		return nil, false
	}
	cut := -1
	n := 0
	i := skipSpace(body, arr+1)
	for i < len(body) && body[i] != ']' {
		end := skipValue(body, i)
		if end <= i {
			return nil, false
		}
		if n > 0 && body[i] == '{' && roleIs(body[:end], i, assistant) {
			cut = i
		}
		n++
		if i = skipSpace(body, end); i < len(body) && body[i] == ',' {
			i = skipSpace(body, i+1)
		} else if i >= len(body) || body[i] != ']' {
			return nil, false
		}
	}
	if i >= len(body) || cut < 0 {
		return nil, false
	}
	out := make([]byte, 0, arr+1+len(body)-cut)
	out = append(out, body[:arr+1]...)
	return append(out, body[cut:]...), true
}

// roleIs reports whether the object at b[i] (ending at len(b)) has exactly
// one top-level "role" key whose raw value is the string want.
func roleIs(b []byte, i int, want string) bool {
	found, match := 0, false
	ok := members(b, i, func(k []byte, v, end int) bool {
		if string(k) == "role" {
			found++
			match = string(b[v:end]) == `"`+want+`"`
		}
		return true
	})
	return ok && found == 1 && match
}

// members walks the object at b[i], calling f with each key's raw text and
// the start and end of its value; false when the object is malformed or f
// stops the walk.
func members(b []byte, i int, f func(k []byte, v, end int) bool) bool {
	if i >= len(b) || b[i] != '{' {
		return false
	}
	i = skipSpace(b, i+1)
	for i < len(b) && b[i] != '}' {
		if b[i] != '"' {
			return false
		}
		ks := i
		kend := skipString(b, ks)
		if kend < 0 {
			return false
		}
		if i = skipSpace(b, kend); i >= len(b) || b[i] != ':' {
			return false
		}
		v := skipSpace(b, i+1)
		end := skipValue(b, v)
		if end <= v || !f(b[ks+1:kend-1], v, end) {
			return false
		}
		if i = skipSpace(b, end); i < len(b) && b[i] == ',' {
			i = skipSpace(b, i+1)
		} else if i >= len(b) || b[i] != '}' {
			return false
		}
	}
	return i < len(b)
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// skipString returns the index after the string opening at b[i], or -1
// when it is not closed. A quote after an odd run of backslashes is
// escaped; each run is counted once, so the scan stays linear.
func skipString(b []byte, i int) int {
	for i++; i < len(b); {
		j := bytes.IndexByte(b[i:], '"')
		if j < 0 {
			return -1
		}
		q := i + j
		n := 0
		for k := q - 1; k >= i && b[k] == '\\'; k-- {
			n++
		}
		if n%2 == 0 {
			return q + 1
		}
		i = q + 1
	}
	return -1
}

// structural marks the bytes skipValue stops at inside a container.
var structural = [256]bool{'"': true, '{': true, '}': true, '[': true, ']': true}

// skipValue returns the index after the value at b[i], or -1 when it is
// not closed. Scalars run to the next delimiter.
func skipValue(b []byte, i int) int {
	if i >= len(b) {
		return -1
	}
	switch b[i] {
	case '"':
		return skipString(b, i)
	case '{', '[':
		depth := 0
		for i < len(b) {
			for i < len(b) && !structural[b[i]] {
				i++
			}
			if i == len(b) {
				return -1
			}
			switch b[i] {
			case '"':
				if i = skipString(b, i); i < 0 {
					return -1
				}
				continue
			case '{', '[':
				depth++
			default:
				if depth--; depth == 0 {
					return i + 1
				}
			}
			i++
		}
		return -1
	}
	start := i
	for i < len(b) && !strings.ContainsRune(",]}: \t\n\r", rune(b[i])) {
		i++
	}
	if i == start {
		return -1
	}
	return i
}
