package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The update path used to keep its own copy of the hook-type list, which fell
// behind the model's list and rejected portal_ui / resource_provider plugins
// ("invalid hook type: resource_provider") whenever their config was saved.
func TestIsValidHookTypeMatchesModelList(t *testing.T) {
	for _, hookType := range models.GetValidHookTypes() {
		if !isValidHookType(hookType) {
			t.Errorf("hook type %q is valid in models but rejected by the plugin service", hookType)
		}
	}
	for _, hookType := range []string{models.HookTypePortalUI, models.HookTypeResourceProvider} {
		if !isValidHookType(hookType) {
			t.Errorf("hook type %q must be accepted on plugin update", hookType)
		}
	}
	if isValidHookType("not_a_hook") {
		t.Error("unknown hook type must be rejected")
	}
}
