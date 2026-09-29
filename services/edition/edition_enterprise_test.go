//go:build enterprise

package edition_test

import (
	"testing"

	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/all"

	"github.com/TykTechnologies/midsommar/v2/services/edition"
)

// TestCheckRegisteredPassesWithEveryFeatureImported fails when a new
// enterprise feature is added to the check but not to enterprise/all.
func TestCheckRegisteredPassesWithEveryFeatureImported(t *testing.T) {
	if err := edition.CheckRegistered(); err != nil {
		t.Fatal(err)
	}
}
