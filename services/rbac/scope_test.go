package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceScopeID_Canonical(t *testing.T) {
	assert.Equal(t, "7", WorkspaceScopeID(7))
	for _, ok := range []string{"1", "7", "4294967295"} {
		id, valid := ParseWorkspaceScopeID(ok)
		assert.True(t, valid, ok)
		assert.Equal(t, ok, WorkspaceScopeID(id))
	}
	for _, bad := range []string{"", "0", "00", "01", " 1", "1 ", "+1", "-1", "1.0", "0x1", "abc", "4294967296"} {
		_, valid := ParseWorkspaceScopeID(bad)
		assert.False(t, valid, "%q", bad)
	}
}
