package guardrails

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// EvidenceMasker returns the byte spans of text an excerpt must not show:
// credentials and personal data. The built-in provider registers one; with
// none registered (Community Edition) no excerpt is made.
type EvidenceMasker func(text string) []Span

var (
	maskerMu sync.RWMutex
	masker   EvidenceMasker
)

// RegisterEvidenceMasker sets the masker excerpts go through.
func RegisterEvidenceMasker(m EvidenceMasker) {
	maskerMu.Lock()
	defer maskerMu.Unlock()
	masker = m
}

// CanExcerpt reports whether a masker is registered, so excerpts can be made.
func CanExcerpt() bool {
	maskerMu.RLock()
	defer maskerMu.RUnlock()
	return masker != nil
}

// TextSHA256 is the hex SHA-256 of text, as providers that log by hash
// (Wolf Defender) key their records.
func TextSHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// ExcerptMask is what an excerpt shows in place of a masked span.
const ExcerptMask = "[masked]"

// Excerpt returns at most limit characters of text around span (the start
// of text when span is nil or out of range), with every span the masker
// reports replaced by ExcerptMask and "…" where text was cut. A masked span
// that crosses the cut is masked whole, so a cut never leaves part of a
// credential showing. It returns "" and false when no masker is registered.
func Excerpt(text string, span *Span, limit int) (string, bool) {
	maskerMu.RLock()
	m := masker
	maskerMu.RUnlock()
	if m == nil {
		return "", false
	}
	if limit <= 0 {
		limit = DefaultExcerptChars
	}
	text = strings.ToValidUTF8(text, "�")

	// The window, in bytes: the span (or its first limit characters), with
	// the rest of the limit shared before and after it.
	s, e := 0, 0
	if span != nil && span.Start >= 0 && span.Start <= span.End && span.End <= len(text) {
		s, e = snapBack(text, span.Start), snapBack(text, span.End)
	}
	if n := utf8.RuneCountInString(text[s:e]); n >= limit {
		e = advance(text, s, limit)
	} else {
		// Half the room before the span, the rest after; what one side
		// cannot use (the start or end of text) goes to the other.
		room := limit - n
		s0 := s
		s = retreat(text, s, room/2)
		used := utf8.RuneCountInString(text[s:s0])
		e0 := e
		e = advance(text, e, room-used)
		used += utf8.RuneCountInString(text[e0:e])
		s = retreat(text, s, room-used)
	}

	masks := mergeSpans(m(text), len(text))
	// Widen the window over any mask that crosses its edges.
	for _, mk := range masks {
		if mk.Start < s && mk.End > s {
			s = snapBack(text, mk.Start)
		}
		if mk.Start < e && mk.End > e {
			e = snapForward(text, mk.End)
		}
	}

	var b strings.Builder
	if s > 0 {
		b.WriteString("…")
	}
	at := s
	for _, mk := range masks {
		if mk.End <= at || mk.Start >= e {
			continue
		}
		start := max(mk.Start, at)
		b.WriteString(text[at:snapBack(text, start)])
		b.WriteString(ExcerptMask)
		at = snapForward(text, min(mk.End, e))
	}
	if at < e {
		b.WriteString(text[at:e])
	}
	if e < len(text) {
		b.WriteString("…")
	}
	return b.String(), true
}

// mergeSpans drops spans outside text and merges the overlapping or
// touching ones, in order.
func mergeSpans(spans []Span, n int) []Span {
	valid := make([]Span, 0, len(spans))
	for _, sp := range spans {
		if sp.Start >= 0 && sp.Start < sp.End && sp.End <= n {
			valid = append(valid, sp)
		}
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Start < valid[j].Start })
	out := valid[:0]
	for _, sp := range valid {
		if k := len(out) - 1; k >= 0 && sp.Start <= out[k].End {
			out[k].End = max(out[k].End, sp.End)
			continue
		}
		out = append(out, sp)
	}
	return out
}

// snapBack moves i back to the start of the character it falls in.
func snapBack(text string, i int) int {
	for i > 0 && i < len(text) && !utf8.RuneStart(text[i]) {
		i--
	}
	return i
}

// snapForward moves i forward to the start of the next character.
func snapForward(text string, i int) int {
	for i < len(text) && !utf8.RuneStart(text[i]) {
		i++
	}
	return min(i, len(text))
}

// advance returns the byte offset n characters after i.
func advance(text string, i, n int) int {
	for n > 0 && i < len(text) {
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
		n--
	}
	return i
}

// retreat returns the byte offset n characters before i.
func retreat(text string, i, n int) int {
	for n > 0 && i > 0 {
		_, size := utf8.DecodeLastRuneInString(text[:i])
		i -= size
		n--
	}
	return i
}
