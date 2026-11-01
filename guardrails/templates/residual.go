package templates

import "strings"

// Residual is what a guardrail scores of a matched message: its text with
// each exempt constant replaced by Placeholder, everything else (slots, and
// constants that are not exempt) in its original order.
type Residual struct {
	// Text is the residual.
	Text string
	// Original is the message as sent.
	Original string
	// Exempt are the exempt constants' byte ranges in Original, in order.
	Exempt []Span
}

// NewResidual builds the residual of text for the exempt ranges, which must
// be in order and not overlap (Match.Exempt).
func NewResidual(text string, exempt []Span) Residual {
	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	for _, e := range exempt {
		b.WriteString(text[prev:e.Start])
		b.WriteString(Placeholder)
		prev = e.End
	}
	b.WriteString(text[prev:])
	return Residual{Text: b.String(), Original: text, Exempt: exempt}
}

// Restore maps a rewrite of the residual (a redaction) back onto the
// original: the rewritten text between placeholders, with each placeholder
// replaced by the constant it stood for. ok is false when the rewrite does
// not hold exactly one placeholder per exempt constant (the provider
// rewrote a placeholder, or wrote one), since its pieces could then not be
// placed; the caller must not apply it.
func (r Residual) Restore(rewritten string) (string, bool) {
	if len(r.Exempt) == 0 {
		return rewritten, true
	}
	pieces := strings.Split(rewritten, Placeholder)
	if len(pieces) != len(r.Exempt)+1 {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(rewritten) + len(r.Original) - len(r.Text))
	for i, e := range r.Exempt {
		b.WriteString(pieces[i])
		b.WriteString(r.Original[e.Start:e.End])
	}
	b.WriteString(pieces[len(pieces)-1])
	return b.String(), true
}

// MapSpan maps a byte range of the residual onto the original. An end that
// falls inside a placeholder extends to the end of its constant, a start to
// its start, so the mapped range covers whatever the placeholder stood for.
func (r Residual) MapSpan(s Span) Span {
	return Span{Start: r.mapOffset(s.Start, false), End: r.mapOffset(s.End, true)}
}

// mapOffset maps one residual offset. Inside a placeholder it maps to the
// constant's end when end is set, else to its start.
func (r Residual) mapOffset(off int, end bool) int {
	shift := 0 // original offset minus residual offset so far
	resPos := 0
	prev := 0
	for _, e := range r.Exempt {
		gapEnd := resPos + (e.Start - prev) // residual offset where the placeholder starts
		if off <= gapEnd {
			return off + shift
		}
		phEnd := gapEnd + len(Placeholder)
		if off < phEnd {
			if end {
				return e.End
			}
			return e.Start
		}
		shift += (e.End - e.Start) - len(Placeholder)
		resPos = phEnd
		prev = e.End
	}
	return min(off+shift, len(r.Original))
}
