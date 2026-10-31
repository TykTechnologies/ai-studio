package templates

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// The exemption cap: at most MaxExemptShare of a message's characters, and
// at most MaxExemptChars, are left unscored. A constant that would take the
// message past either is scored.
const (
	MaxExemptPercent = 60
	MaxExemptChars   = 16 << 10
)

// maxOccurrences bounds how often one constant may occur in a message. More
// copies than that are not a prompt the app wrote: the template does not
// match (and the copies are a mimicry signal), which also bounds the
// matcher's work on text built to make it slow.
const maxOccurrences = 64

// shingleBytes is the window the mimicry check looks for: any run of this
// many bytes of a constant found inside a slot is template text in a slot.
const shingleBytes = MinConstBytes

// Message is one message a template may match: its role, its text, and
// whether it is its role's first and last message in the request.
type Message struct {
	Role        string
	Text        string
	First, Last bool
}

// Span is a byte range [Start, End) of a text.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Match is a template matched on a message.
type Match struct {
	Template *Template
	// Consts are where the template's constants are in the message, in
	// order.
	Consts []Span
	// Exempt are the constants left unscored, in order: those not refused
	// for an invisible neighbour and within the exemption cap.
	Exempt []Span
	// Refused counts constants scored because an invisible character or a
	// combining mark touches them; Capped those the exemption cap kept.
	Refused, Capped int
	// Mimicry reports template text inside a slot. The slot is scored
	// as always; the signal is for the operator.
	Mimicry bool
	// ExemptChars and TotalChars are the characters exempt and in the
	// whole message.
	ExemptChars, TotalChars int
}

// ExemptShare is the share of the message's characters left unscored.
func (m *Match) ExemptShare() float64 {
	if m == nil || m.TotalChars == 0 {
		return 0
	}
	return float64(m.ExemptChars) / float64(m.TotalChars)
}

// Outcome is what matching a message against an app's templates found.
type Outcome struct {
	// Match is the template that matched, nil for none.
	Match *Match
	// Fits reports that some template's placement covers the message, so
	// a nil Match is a miss.
	Fits bool
	// Mimicry reports template text where it does not belong: inside a slot
	// of the match, or (with no match) the placeholder or too many copies of
	// a constant in a message a template fits.
	Mimicry bool
}

// Fits reports whether the placement covers msg.
func (p Placement) Fits(msg Message) bool {
	if p.Role != msg.Role {
		return false
	}
	switch p.Message {
	case MessageFirst:
		return msg.First
	case MessageLast:
		return msg.Last
	}
	return p.Message == MessageAny
}

// MatchMessage matches msg against the templates. Every constant of a
// template must appear byte for byte, in order, at the anchored position;
// there is no normalisation of any kind. When several templates, or several
// parses of one, match, the one that leaves the most text scored wins (ties
// go to the earlier template, and the leftmost parse). Anything short of a
// match is no match: the caller scores the whole message.
func MatchMessage(ts []*Template, msg Message) Outcome {
	var out Outcome
	if !utf8.ValidString(msg.Text) {
		for _, t := range ts {
			out.Fits = out.Fits || t.Placement.Fits(msg)
		}
		return out
	}
	total := utf8.RuneCountInString(msg.Text)
	for _, t := range ts {
		if t == nil || !t.Placement.Fits(msg) {
			continue
		}
		out.Fits = true
		c := t.compiled()
		if c == nil {
			continue // not a valid template: never trusted
		}
		m, mimicry := c.match(msg.Text, total)
		out.Mimicry = out.Mimicry || mimicry
		if m == nil {
			continue
		}
		m.Template = t
		if out.Match == nil || m.ExemptChars < out.Match.ExemptChars {
			out.Match = m
		}
	}
	return out
}

// compiledTemplate is a template prepared for matching.
type compiledTemplate struct {
	consts []string
	// lead and trail report a slot (explicit, or implied by the anchor)
	// before the first constant and after the last.
	lead, trail bool
	// shingles holds every shingleBytes-long run of every constant, by
	// rolling hash, for the mimicry check.
	shingles map[uint64]string
}

func (t *Template) compiled() *compiledTemplate {
	t.once.Do(func() { t.c = compile(t) })
	return t.c
}

func compile(t *Template) *compiledTemplate {
	if Validate(t.Placement, t.Parts) != nil {
		return nil
	}
	c := &compiledTemplate{shingles: map[uint64]string{}}
	for _, p := range t.Parts {
		if p.IsConst() {
			c.consts = append(c.consts, p.Const)
		}
	}
	c.lead = !t.Parts[0].IsConst()
	c.trail = !t.Parts[len(t.Parts)-1].IsConst()
	if t.Anchor == AnchorEnd {
		c.lead = true
	}
	if t.Anchor == AnchorStart {
		c.trail = true
	}
	for _, k := range c.consts {
		for i := 0; i+shingleBytes <= len(k); i++ {
			w := k[i : i+shingleBytes]
			c.shingles[hashWindow(w)] = w
		}
	}
	return c
}

// occurrence is one place a constant appears.
type occurrence struct {
	start, end int
	// cost is the bytes exempt if this occurrence is chosen: 0 when an
	// invisible character touches it, so it would be scored anyway.
	cost int
	// best is the least exempt bytes of a parse of this constant and the
	// ones after it, starting here; -1 when there is none.
	best int
}

// match finds the parse of text that leaves the most of it scored. mimicry
// reports template text where it does not belong, match or not.
func (c *compiledTemplate) match(text string, total int) (*Match, bool) {
	n := len(c.consts)
	occ := make([][]occurrence, n)
	const mimicry = false
	for k, s := range c.consts {
		for from := 0; from <= len(text)-len(s); {
			i := strings.Index(text[from:], s)
			if i < 0 {
				break
			}
			at := from + i
			from = at + 1
			if k == 0 && !c.lead && at != 0 {
				continue
			}
			if k == n-1 && !c.trail && at+len(s) != len(text) {
				continue
			}
			if len(occ[k]) == maxOccurrences {
				return nil, true // far more copies than any prompt holds
			}
			o := occurrence{start: at, end: at + len(s), cost: len(s)}
			if touched(text, at, at+len(s)) {
				o.cost = 0
			}
			occ[k] = append(occ[k], o)
		}
		if len(occ[k]) == 0 {
			return nil, false
		}
	}

	// From the last constant back: the least exempt bytes of a parse that
	// starts with each occurrence, given the occurrences of the next
	// constant that start at or after its end.
	for k := n - 1; k >= 0; k-- {
		for j := range occ[k] {
			o := &occ[k][j]
			if k == n-1 {
				o.best = o.cost
				continue
			}
			o.best = -1
			if rest := minBestFrom(occ[k+1], o.end); rest >= 0 {
				o.best = o.cost + rest
			}
		}
	}
	// Forward: at each constant, the leftmost occurrence that achieves the
	// least, so ties go to the leftmost parse.
	want := -1
	for _, o := range occ[0] {
		if o.best >= 0 && (want < 0 || o.best < want) {
			want = o.best
		}
	}
	if want < 0 {
		return nil, mimicry // no parse
	}
	chosen := make([]occurrence, 0, n)
	after := 0
	for k := range n {
		found := false
		for _, o := range occ[k] {
			if o.start >= after && o.best == want {
				chosen = append(chosen, o)
				want, after, found = want-o.cost, o.end, true
				break
			}
		}
		if !found {
			return nil, mimicry
		}
	}

	m := &Match{TotalChars: total}
	slots := make([]Span, 0, n+1)
	prev := 0
	for _, o := range chosen {
		m.Consts = append(m.Consts, Span{o.start, o.end})
		slots = append(slots, Span{prev, o.start})
		prev = o.end
	}
	slots = append(slots, Span{prev, len(text)})
	for _, s := range slots {
		slot := text[s.Start:s.End]
		if strings.Contains(slot, Placeholder) {
			// The residual could not be told apart from the slot's own
			// text: never exempt, score the message whole.
			return nil, true
		}
		if c.inSlot(slot) {
			m.Mimicry = true
		}
	}
	m.Mimicry = m.Mimicry || mimicry

	// The exemption cap, constant by constant in order.
	limit := min(total*MaxExemptPercent/100, MaxExemptChars)
	for _, o := range chosen {
		if o.cost == 0 {
			m.Refused++
			continue
		}
		chars := utf8.RuneCountInString(text[o.start:o.end])
		if m.ExemptChars+chars > limit {
			m.Capped++
			continue
		}
		m.ExemptChars += chars
		m.Exempt = append(m.Exempt, Span{o.start, o.end})
	}
	return m, m.Mimicry
}

// minBestFrom is the least best of the occurrences starting at or after
// pos, -1 when none has a parse.
func minBestFrom(occ []occurrence, pos int) int {
	i := sort.Search(len(occ), func(i int) bool { return occ[i].start >= pos })
	least := -1
	for _, o := range occ[i:] {
		if o.best >= 0 && (least < 0 || o.best < least) {
			least = o.best
		}
	}
	return least
}

// touched reports an invisible character, or a combining mark that would
// join the constant's last character, right next to text[start:end].
func touched(text string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(text[:start]); Invisible(r) {
			return true
		}
	}
	if end < len(text) {
		if r, _ := utf8.DecodeRuneInString(text[end:]); Invisible(r) || combining(r) {
			return true
		}
	}
	return false
}

// inSlot reports a shingleBytes-long run of a constant inside slot.
func (c *compiledTemplate) inSlot(slot string) bool {
	if len(slot) < shingleBytes {
		return false
	}
	h := hashWindow(slot[:shingleBytes])
	for i := 0; ; i++ {
		if w, ok := c.shingles[h]; ok && w == slot[i:i+shingleBytes] {
			return true
		}
		if i+shingleBytes >= len(slot) {
			return false
		}
		h = rollWindow(h, slot[i], slot[i+shingleBytes])
	}
}

// Rabin-Karp over shingleBytes-byte windows, modulo 2^64. A collision is
// checked against the window itself, so it costs a comparison, not a false
// signal.
const hashBase = 1099511628211

var hashTop = func() uint64 {
	p := uint64(1)
	for range shingleBytes - 1 {
		p *= hashBase
	}
	return p
}()

func hashWindow(w string) uint64 {
	var h uint64
	for i := 0; i < len(w); i++ {
		h = h*hashBase + uint64(w[i])
	}
	return h
}

func rollWindow(h uint64, out, in byte) uint64 {
	return (h-uint64(out)*hashTop)*hashBase + uint64(in)
}
