package filterscript

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The startup warning fires for every spelling the Enterprise runner treats
// as on, and for nothing else.
func TestOSModuleAllowed(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", " yes ", "On"} {
		t.Setenv(EnvScriptAllowOS, v)
		assert.True(t, OSModuleAllowed(), "%q", v)
	}
	for _, v := range []string{"", "0", "false", "off", "no", "maybe"} {
		t.Setenv(EnvScriptAllowOS, v)
		assert.False(t, OSModuleAllowed(), "%q", v)
	}
}
