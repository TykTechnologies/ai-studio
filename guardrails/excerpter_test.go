package guardrails

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// excerptReference is Excerpt as it was before the Excerpter (4qm): the
// masker and the window over the whole text for every excerpt. The
// Excerpter must give exactly what it gave.
func excerptReference(text string, span *Span, limit int) (string, bool) {
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
	s, e := 0, 0
	if span != nil && span.Start >= 0 && span.Start <= span.End && span.End <= len(text) {
		s, e = snapBack(text, span.Start), snapBack(text, span.End)
	}
	if n := utf8.RuneCountInString(text[s:e]); n >= limit {
		e = advance(text, s, limit)
	} else {
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

// The Excerpter gives exactly the excerpts Excerpt gave before it, for any
// span (in range, out of range, inside or across masks, longer than the
// limit), limit and text, multi-byte and invalid UTF-8 included.
func TestExcerpter_MatchesReference(t *testing.T) {
	withMasker(t, "secret-token", "4111 1111", "é€x")
	rng := rand.New(rand.NewSource(4))
	words := []string{"ignore ", "previous ", "secret-token", "4111 1111", " é€x ", "naïve ", "\xff\xfe", "instructions. ", "日本語 "}
	for round := 0; round < 200; round++ {
		var b strings.Builder
		for n := rng.Intn(80); n >= 0; n-- {
			b.WriteString(words[rng.Intn(len(words))])
		}
		text := b.String()
		x := NewExcerpter(text)
		for k := 0; k < 30; k++ {
			var span *Span
			if rng.Intn(5) > 0 {
				a, c := rng.Intn(len(text)+3)-1, rng.Intn(len(text)+3)-1
				if a > c && rng.Intn(2) == 0 {
					a, c = c, a
				}
				span = &Span{Start: a, End: c}
			}
			limit := rng.Intn(60) - 2
			want, wok := excerptReference(text, span, limit)
			got, gok := x.Excerpt(span, limit)
			if got != want || gok != wok {
				t.Fatalf("text %q span %+v limit %d:\n got  %q\n want %q", text, span, limit, got, want)
			}
		}
	}
}
