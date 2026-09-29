//go:build studio_noui

package ui

import "testing"

func TestNoUIBuildEmbedsOnlyThePlaceholder(t *testing.T) {
	if Embedded {
		t.Fatal("a studio_noui build reports an embedded frontend")
	}
}
