package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStr(t *testing.T) {
	assert.Equal(t, "user:1", UserStr(1))
	assert.Equal(t, "user:42", UserStr(42))
}

func TestGroupStr(t *testing.T) {
	assert.Equal(t, "group:5", GroupStr(5))
}

func TestGroupMemberStr(t *testing.T) {
	assert.Equal(t, "group:5#member", GroupMemberStr(5))
}

func TestObjectStr(t *testing.T) {
	assert.Equal(t, "catalogue:3", ObjectStr("catalogue", 3))
	assert.Equal(t, "llm:100", ObjectStr("llm", 100))
}

func TestParseObjectID(t *testing.T) {
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
			id, err := ParseObjectID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}
