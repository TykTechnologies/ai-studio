package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jyx.175: owner_may_view_content is not available until the Portal trace
// views. Setting it is refused, and a value stored earlier reads as false.
func TestAppTracingOwnerContentUnavailable(t *testing.T) {
	assert.ErrorContains(t, AppTracingSettings{OwnerMayViewContent: true}.Validate(), "owner_may_view_content")
	assert.NoError(t, AppTracingSettings{ContentMode: "redacted"}.Validate())

	var s AppTracingSettings
	require.NoError(t, s.Scan(`{"content_mode":"redacted","owner_may_view_content":true}`))
	assert.False(t, s.OwnerMayViewContent)
	assert.Equal(t, "redacted", s.ContentMode)
}
