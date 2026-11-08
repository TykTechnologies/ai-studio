package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func llmRequest(name string, active bool) *CreateLLMRequest {
	return &CreateLLMRequest{Name: name, Vendor: "openai", DefaultModel: "gpt-4", APIKey: "sk-test", IsActive: active}
}

func fixedLimit(max int64, unlimited bool) LLMLimitFunc {
	return func() (int64, bool) { return max, unlimited }
}

// Every LLM not deleted counts, inactive ones included; a refused create
// stores nothing and deleting an LLM frees its slot.
func TestCreateLLMWithinLicensedLimit(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	service.SetLLMLimit(fixedLimit(2, false))

	first, err := service.CreateLLM(llmRequest("one", true))
	require.NoError(t, err)
	_, err = service.CreateLLM(llmRequest("two", false))
	require.NoError(t, err, "an inactive LLM takes a slot too")

	_, err = service.CreateLLM(llmRequest("three", true))
	require.Error(t, err)
	assert.True(t, IsLLMLimitError(err))
	assert.Equal(t, "Your organisation has reached its licensed limit of 2 LLMs. Remove an LLM to make room, or contact your administrator to increase the quota.", err.Error())
	n, err := countLLMs(db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	require.NoError(t, service.DeleteLLM(first.ID))
	_, err = service.CreateLLM(llmRequest("three", true))
	require.NoError(t, err, "deleting an LLM frees its slot")
}

func TestCreateLLMUnlimitedOrUnset(t *testing.T) {
	_, _, service := setupManagementTestDB(t)
	for i := 0; i < 3; i++ {
		_, err := service.CreateLLM(llmRequest(fmt.Sprintf("unset-%d", i), true))
		require.NoError(t, err, "no limit set (Community Edition)")
	}
	service.SetLLMLimit(fixedLimit(0, true))
	_, err := service.CreateLLM(llmRequest("unlimited", true))
	require.NoError(t, err)
}

// A licence without max_llms (0) allows none; after a downgrade below the
// current count the existing LLMs stay and new ones are refused.
func TestCreateLLMZeroAndDowngrade(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	service.SetLLMLimit(fixedLimit(0, false))
	_, err := service.CreateLLM(llmRequest("none", true))
	assert.True(t, IsLLMLimitError(err), "got %v", err)
	assert.Contains(t, err.Error(), "limit of 0 LLMs.")

	service.SetLLMLimit(fixedLimit(0, true))
	for i := 0; i < 3; i++ {
		_, err := service.CreateLLM(llmRequest(fmt.Sprintf("d-%d", i), true))
		require.NoError(t, err)
	}
	service.SetLLMLimit(fixedLimit(1, false))
	_, err = service.CreateLLM(llmRequest("more", true))
	assert.True(t, IsLLMLimitError(err))
	assert.Contains(t, err.Error(), "limit of 1 LLM.")
	n, err := countLLMs(db)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n, "nothing is revoked")
}
