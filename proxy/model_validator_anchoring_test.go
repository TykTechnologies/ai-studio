package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Allowed Models patterns are matched unanchored, which surprises people: the
// example the UI itself suggests, "gpt-4.*", also permits "legacy-gpt-4o".
// These pin the behaviour so a future change to it is deliberate rather than
// accidental, and so the documented semantics stay true.
func TestIsModelAllowed_PatternsAreUnanchored(t *testing.T) {
	v := NewModelValidator([]string{"gpt-4.*"})

	assert.True(t, v.IsModelAllowed("gpt-4o"), "the intended match")
	assert.True(t, v.IsModelAllowed("legacy-gpt-4o"),
		"matched anywhere in the name -- this is the surprise the UI now warns about")
	assert.True(t, v.IsModelAllowed("not-really-gpt-4-either"))
	assert.False(t, v.IsModelAllowed("claude-sonnet-4-5-20250929"))
}

func TestIsModelAllowed_AnchoringWorksWhenAsked(t *testing.T) {
	v := NewModelValidator([]string{"^gpt-4.*$"})

	assert.True(t, v.IsModelAllowed("gpt-4o"))
	assert.False(t, v.IsModelAllowed("legacy-gpt-4o"),
		"anchoring is how you get whole-name matching")
}

func TestIsModelAllowed_EmptyListAllowsEverything(t *testing.T) {
	// An empty Allowed Models field is not a deny-all, which is worth stating
	// on screen rather than leaving as an empty field.
	v := NewModelValidator(nil)

	assert.True(t, v.IsModelAllowed("anything-at-all"))
	assert.True(t, v.IsModelAllowed(""))
}

func TestIsModelAllowed_InvalidPatternDoesNotAllowEverything(t *testing.T) {
	// A malformed regex must not become a wildcard.
	v := NewModelValidator([]string{"[unclosed"})

	assert.False(t, v.IsModelAllowed("gpt-4o"))
}
