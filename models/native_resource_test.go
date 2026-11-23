package models

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNativeResource_RequestedModelFor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested []string
		reported  string
		want      string
	}{
		{"row without requested models keeps the reported name", nil, "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001"},
		{"the requested model itself", []string{"claude-luna", "claude-sol"}, "claude-sol", "claude-sol"},
		{"alias answered with its dated snapshot", []string{"claude-haiku-4-5"}, "claude-haiku-4-5-20251001", "claude-haiku-4-5"},
		{"dated snapshot among several requested models", []string{"claude-sonnet-4-5", "claude-haiku-4-5"}, "claude-haiku-4-5-20251001", "claude-haiku-4-5"},
		{"the longest requested prefix wins", []string{"gpt-4o", "gpt-4o-mini"}, "gpt-4o-mini-2024-07-18", "gpt-4o-mini"},
		{"a prefix must end at a separator", []string{"gpt-4", "gpt-5"}, "gpt-4o-2024-08-06", "gpt-4o-2024-08-06"},
		{"single requested alias without a common prefix", []string{"claude-3-5-sonnet-latest"}, "claude-3-5-sonnet-20241022", "claude-3-5-sonnet-latest"},
		{"several requested models and no match is not guessed", []string{"claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"}, "claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20241022"},
		{"no reported model, one requested", []string{"gpt-4o"}, "", "gpt-4o"},
		{"no reported model, several requested", []string{"gpt-4o", "gpt-4o-mini"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := &NativeResource{}
			row.SetRequestedModels(tc.requested)
			assert.Equal(t, tc.want, row.RequestedModelFor(tc.reported))
		})
	}
}

func TestNativeResource_SetRequestedModels(t *testing.T) {
	row := &NativeResource{}
	row.SetRequestedModels([]string{"b", "", "a", "b", "a"})
	assert.Equal(t, `["b","a"]`, row.RequestedModels, "distinct, non-empty, in order of first use")
	assert.Equal(t, []string{"b", "a"}, row.RequestedModelList())

	row.SetRequestedModels(nil)
	assert.Empty(t, row.RequestedModels)
	assert.Nil(t, row.RequestedModelList())

	var many []string
	for i := 0; i <= NativeRequestedModelsMax; i++ {
		many = append(many, fmt.Sprintf("model-%d", i))
	}
	row.SetRequestedModels(many)
	assert.Empty(t, row.RequestedModels, "over the cap nothing is kept")
	row.SetRequestedModels(many[:NativeRequestedModelsMax])
	assert.Len(t, row.RequestedModelList(), NativeRequestedModelsMax)

	row.RequestedModels = "not json"
	assert.Nil(t, row.RequestedModelList())
	assert.Equal(t, "x", row.RequestedModelFor("x"))
	assert.Equal(t, "x", (*NativeResource)(nil).RequestedModelFor("x"))
}
