package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubjectUser(t *testing.T) {
	assert.Equal(t, "user:1", SubjectUser(1))
	assert.Equal(t, "user:42", SubjectUser(42))
}

func TestSubjectGroup(t *testing.T) {
	assert.Equal(t, "group:5", SubjectGroup(5))
}

func TestResourceID(t *testing.T) {
	assert.Equal(t, "catalogue:3", ResourceID("catalogue", 3))
	assert.Equal(t, "llm:100", ResourceID("llm", 100))
}

func TestResourceByName(t *testing.T) {
	// Valid IDs
	res, err := ResourceByName("plugin_resource", "5_srv-1")
	require.NoError(t, err)
	assert.Equal(t, "plugin_resource:5_srv-1", res)

	// Colons are rejected
	_, err = ResourceByName("plugin_resource", "evil:123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden colon")

	// Empty IDs are rejected
	_, err = ResourceByName("plugin_resource", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty ID")
}

func TestParseResourceNumericID(t *testing.T) {
	tests := []struct {
		input   string
		wantID  uint
		wantErr bool
	}{
		{"llm:42", 42, false},
		{"catalogue:1", 1, false},
		{"data_catalogue:999", 999, false},
		{"plugin_resource:5_srv-1", 0, true}, // non-numeric after colon
		{"invalid", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, err := ParseResourceNumericID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

func TestParseResourceID(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"llm:42", "42", false},
		{"plugin_resource:5_srv-1", "5_srv-1", false},
		{"system:1", "1", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseResourceID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestValidateID_RejectsColons(t *testing.T) {
	assert.NoError(t, ValidateID("5_srv-1"))
	assert.NoError(t, ValidateID("42"))
	assert.Error(t, ValidateID("evil:123"))
	assert.Error(t, ValidateID(""))
}
