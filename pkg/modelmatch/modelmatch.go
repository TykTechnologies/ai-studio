// Package modelmatch is the single implementation of the LLM "allowed models"
// matching rule. The proxy (request time), the LLM service (save time) and the
// failover validator all delegate here so a model that passes in one place
// passes everywhere.
package modelmatch

import (
	"fmt"
	"regexp"
)

// Allowed reports whether modelName is permitted by the configured patterns.
//
// Two behaviours worth stating plainly, because both surprise people:
//
//  1. An empty pattern list allows EVERY model. An empty Allowed Models field
//     is not a deny-all.
//  2. Patterns are matched UNANCHORED, i.e. anywhere within the model name. So
//     "gpt-4.*" -- the example the UI itself suggests -- also matches
//     "legacy-gpt-4o" and "not-really-gpt-4". Anchor explicitly with ^ and $
//     if you mean the whole name.
//
// The unanchored behaviour is kept deliberately: existing configurations rely
// on substring matching, and silently anchoring them would start rejecting
// models that are allowed today.
//
// A pattern that fails to compile never matches; it does not turn into a
// wildcard. Use AllowedStrict when the caller wants to hear about it.
func Allowed(patterns []string, modelName string) bool {
	if len(patterns) == 0 {
		return true // If no models specified, allow all
	}
	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, modelName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// AllowedStrict is Allowed but surfaces an invalid pattern as an error instead
// of treating it as a non-match. Save-time validation uses this so an admin
// learns about a broken pattern rather than silently locking a model out.
func AllowedStrict(patterns []string, modelName string) (bool, error) {
	if len(patterns) == 0 {
		return true, nil // Empty list means all models are allowed
	}
	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, modelName)
		if err != nil {
			return false, fmt.Errorf("invalid pattern '%s': %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
