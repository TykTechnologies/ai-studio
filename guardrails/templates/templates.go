// Package templates holds trusted prompt templates: app-level prompt
// templates an operator has approved, whose constant text request guardrails
// do not score (features/TrustedPromptTemplates.md).
//
// A template is an ordered list of parts, each either constant text the app
// always sends or a slot whose content varies. A message matches when every
// constant appears in it byte for byte, in order, at the anchored position;
// the guardrail then scores the residual: the slots in order with a
// placeholder where each exempt constant was. Anything short of an exact
// match leaves the message to be scored whole, as if no template existed.
//
// The package is pure: it holds the types, their validation, the matcher and
// the residual mapping. Storage lives in models and services, and the
// guardrail pipeline (scripting) applies a match to a request.
package templates

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Limits on templates.
const (
	// MinConstBytes is the shortest constant a template may hold. A short
	// constant would exempt text an attacker can easily reproduce, and
	// recurs too often to locate a slot.
	MinConstBytes = 32
	// MaxApprovedPerApp bounds the approved templates of one app.
	MaxApprovedPerApp = 10
	// MaxParts bounds a template's parts.
	MaxParts = 64
	// MaxConstBytes bounds one constant.
	MaxConstBytes = 64 << 10
	// MaxSlotNameLen bounds a slot's name.
	MaxSlotNameLen = 64
)

// Placeholder replaces each exempt constant in the residual a guardrail
// scores, so the classifier still sees that something stood there.
const Placeholder = "[app template]"

// Roles a template may be placed on. Only system and user for now: the
// guardrail segmenter reads an OpenAI developer message as user text, so a
// developer placement could not be told apart from a user one.
const (
	RoleSystem = "system"
	RoleUser   = "user"
)

// Which message of the role a template applies to: the role's first message
// in the request, its last, or any.
const (
	MessageFirst = "first"
	MessageLast  = "last"
	MessageAny   = "any"
)

// Anchoring: the template starts the message, ends it, or both (it is the
// whole message). Text outside an anchored side is an implicit slot.
const (
	AnchorStart = "start"
	AnchorEnd   = "end"
	AnchorBoth  = "both"
)

// Statuses. Only an approved template is ever matched.
const (
	StatusProposed = "proposed"
	StatusApproved = "approved"
	StatusRetired  = "retired"
)

// Part is one part of a template: exactly one of Const (exact text the app
// sends) and Slot (the name of a part that varies).
type Part struct {
	Const string `json:"const,omitempty"`
	Slot  string `json:"slot,omitempty"`
}

// IsConst reports a constant part.
func (p Part) IsConst() bool { return p.Const != "" }

// Placement says which messages a template is tried on.
type Placement struct {
	Role    string `json:"role"`
	Message string `json:"message"`
	Anchor  string `json:"anchor"`
}

// ErrInvalid wraps every validation failure, so a caller can tell the
// user's mistake from anything else.
var ErrInvalid = errors.New("invalid prompt template")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Normalize fills the placement's defaults (message any, anchor both) and
// returns it.
func (p Placement) Normalize() Placement {
	if p.Message == "" {
		p.Message = MessageAny
	}
	if p.Anchor == "" {
		p.Anchor = AnchorBoth
	}
	return p
}

// Validate checks a template's placement and parts. The placement must be
// normalised first.
func Validate(pl Placement, parts []Part) error {
	switch pl.Role {
	case RoleSystem, RoleUser:
	default:
		return invalid("placement.role must be one of system, user")
	}
	switch pl.Message {
	case MessageFirst, MessageLast, MessageAny:
	default:
		return invalid("placement.message must be one of first, last, any")
	}
	switch pl.Anchor {
	case AnchorStart, AnchorEnd, AnchorBoth:
	default:
		return invalid("placement.anchor must be one of start, end, both")
	}
	if len(parts) == 0 {
		return invalid("parts must not be empty")
	}
	if len(parts) > MaxParts {
		return invalid("at most %d parts", MaxParts)
	}
	consts := 0
	for i, p := range parts {
		switch {
		case p.Const != "" && p.Slot != "":
			return invalid("part %d has both const and slot", i)
		case p.Const == "" && p.Slot == "":
			return invalid("part %d has neither const nor slot", i)
		case p.Slot != "":
			if len(p.Slot) > MaxSlotNameLen || strings.TrimSpace(p.Slot) != p.Slot {
				return invalid("part %d: slot name must be 1-%d characters without surrounding spaces", i, MaxSlotNameLen)
			}
			if i > 0 && parts[i-1].Slot != "" {
				return invalid("part %d: two slots in a row cannot be told apart", i)
			}
			continue
		}
		consts++
		if i > 0 && parts[i-1].Const != "" {
			return invalid("part %d: two constants in a row must be one constant", i)
		}
		if err := validConst(p.Const); err != nil {
			return invalid("part %d: %v", i, err)
		}
	}
	if consts == 0 {
		return invalid("a template needs at least one constant")
	}
	if (pl.Anchor == AnchorStart || pl.Anchor == AnchorBoth) && !parts[0].IsConst() {
		return invalid("a template anchored at the start must start with a constant")
	}
	if (pl.Anchor == AnchorEnd || pl.Anchor == AnchorBoth) && !parts[len(parts)-1].IsConst() {
		return invalid("a template anchored at the end must end with a constant")
	}
	return nil
}

// validConst checks one constant: long enough, valid UTF-8, and free of
// invisible characters, which could hide text in what an operator approves.
func validConst(c string) error {
	if len(c) < MinConstBytes {
		return fmt.Errorf("a constant must be at least %d bytes", MinConstBytes)
	}
	if len(c) > MaxConstBytes {
		return fmt.Errorf("a constant must be at most %d bytes", MaxConstBytes)
	}
	if !utf8.ValidString(c) {
		return errors.New("a constant must be valid UTF-8")
	}
	for i, r := range c {
		if Invisible(r) {
			return fmt.Errorf("a constant must not contain invisible characters (U+%04X at byte %d)", r, i)
		}
	}
	if strings.Contains(c, Placeholder) {
		return fmt.Errorf("a constant must not contain the placeholder %q", Placeholder)
	}
	if r, _ := utf8.DecodeRuneInString(c); combining(r) {
		return errors.New("a constant must not start with a combining mark")
	}
	return nil
}

// Invisible reports a character that renders as nothing, or changes how its
// neighbours render, and so can hide or alter text in or next to an approved
// constant: format characters (zero-width characters, bidi controls, the
// Unicode tag block, the BOM, the soft hyphen), variation selectors, the
// combining grapheme joiner, and the fillers some fonts draw blank.
// Ordinary combining marks are not invisible (scripts such as Devanagari
// need them); a constant that starts with one, or that one follows, is
// refused separately, since it would combine with the text around it.
func Invisible(r rune) bool {
	switch {
	case unicode.Is(unicode.Cf, r): // ZW*, bidi controls, tags, BOM, soft hyphen
		return true
	case unicode.Is(unicode.Variation_Selector, r):
		return true
	case r >= 0xE0000 && r <= 0xE0FFF: // tags and supplementary variation selectors, assigned or not
		return true
	}
	switch r {
	case 0x034F, // combining grapheme joiner
		0x115F, 0x1160, 0x3164, 0xFFA0, // Hangul fillers
		0x2800,         // braille blank
		0x180E,         // Mongolian vowel separator
		0xFFFC, 0xFFFD: // object replacement, replacement character
		return true
	}
	return false
}

// combining reports a mark that joins the character before it.
func combining(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r)
}

// Template is an approved template as the matcher uses it.
type Template struct {
	// RecordID is the stored row, for recording matches; 0 in tests.
	RecordID uint
	// Name and Version identify the template in events and on the wire.
	Name    string
	Version int
	Placement
	Parts []Part

	// The matcher's preparation of the template, made on first use.
	once sync.Once
	c    *compiledTemplate
}

// Ref is the template's name@version.
func (t *Template) Ref() string {
	return fmt.Sprintf("%s@%d", t.Name, t.Version)
}
