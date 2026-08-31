package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

// PATCH bound straight onto LLMInput, whose attributes are plain values rather
// than pointers, so an omitted field arrived as its zero value and was written.
// Sending {"filters":[3]} wiped the OpenAI provider: name, vendor, endpoint,
// default model and API key written back empty, and active flipped to false.

func existingLLM() *models.LLM {
	budget := 100.0
	return &models.LLM{
		Name:             "OpenAI",
		APIKey:           "$SECRET/OPENAI_KEY",
		APIEndpoint:      "https://api.openai.com/v1",
		PrivacyScore:     10,
		ShortDescription: "short",
		LongDescription:  "long",
		LogoURL:          "https://example.test/logo.png",
		Vendor:           models.OPENAI,
		Active:           true,
		DefaultModel:     "gpt-5",
		AllowedModels:    []string{"gpt-5", "gpt-5-mini"},
		Namespace:        "default",
		DontLogBodies:    true,
		MonthlyBudget:    &budget,
		Filters:          []*models.Filter{{ID: 7}},
	}
}

func TestMergeLLMPatch_OmittedFieldsAreRetained(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{"filters":[3]}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	existing := existingLLM()
	mergeLLMPatch(&input, existing, llmPatchAttributeKeys(body))

	attrs := input.Data.Attributes
	assert.Equal(t, "OpenAI", attrs.Name, "name must survive a patch that did not mention it")
	assert.Equal(t, "$SECRET/OPENAI_KEY", attrs.APIKey)
	assert.Equal(t, "https://api.openai.com/v1", attrs.APIEndpoint)
	assert.Equal(t, "openai", attrs.Vendor)
	assert.Equal(t, "gpt-5", attrs.DefaultModel)
	assert.Equal(t, []string{"gpt-5", "gpt-5-mini"}, attrs.AllowedModels)
	assert.Equal(t, 10, attrs.PrivacyScore)
	assert.True(t, attrs.Active, "active must not flip to false because it was omitted")
	assert.True(t, attrs.DontLogBodies)
	assert.Equal(t, "default", attrs.Namespace)
	assert.NotNil(t, attrs.MonthlyBudget)

	// The one field that was supplied is the one that changes.
	assert.Equal(t, []uint{3}, attrs.Filters)
}

func TestMergeLLMPatch_SuppliedFieldsWin(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{"name":"Renamed","active":false}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	mergeLLMPatch(&input, existingLLM(), llmPatchAttributeKeys(body))

	assert.Equal(t, "Renamed", input.Data.Attributes.Name)
	// Explicitly sending false must still deactivate: absence and an explicit
	// zero value have to stay distinguishable.
	assert.False(t, input.Data.Attributes.Active)
	// Everything unmentioned is untouched.
	assert.Equal(t, "gpt-5", input.Data.Attributes.DefaultModel)
}

func TestMergeLLMPatch_ExplicitEmptyStringClearsField(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{"short_description":""}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	mergeLLMPatch(&input, existingLLM(), llmPatchAttributeKeys(body))

	assert.Equal(t, "", input.Data.Attributes.ShortDescription,
		"an explicitly supplied empty value must still clear the field")
	assert.Equal(t, "long", input.Data.Attributes.LongDescription)
}

func TestMergeLLMPatch_FiltersRetainedWhenAbsent(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{"name":"Renamed"}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	mergeLLMPatch(&input, existingLLM(), llmPatchAttributeKeys(body))

	assert.Equal(t, []uint{7}, input.Data.Attributes.Filters,
		"filters must not be detached by a patch that did not mention them")
}

func TestLLMPatchAttributeKeys_MalformedBody(t *testing.T) {
	assert.Nil(t, llmPatchAttributeKeys([]byte("not json")))
}

func TestRedactedAPIKeyRetained(t *testing.T) {
	// The API serialises api_key as "[redacted]", so a naive read-modify-write
	// would post the placeholder straight back into the credential field.
	assert.True(t, redactedAPIKeyRetained("[redacted]"))
	assert.False(t, redactedAPIKeyRetained("sk-real-key"))
}

// mergeLLMPatch restores fields by hand, one if-block each, so a field added to
// LLMInput later can silently miss out -- and the failure is invisible until
// someone's provider loses data. budget_start_date was exactly that: omitted
// from the merge, it arrived nil and overwrote the stored value.
//
// This guards the property structurally rather than by maintaining a second
// list that can drift from the first. Populate an existing LLM so that every
// attribute has a non-zero value, send a PATCH that mentions nothing at all,
// and assert that no attribute came back at its zero value. Any unhandled
// field fails here, named.
func TestMergeLLMPatch_EveryFieldIsRestored(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	existing := existingLLM()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	existing.BudgetStartDate = &start
	existing.Metadata = models.JSONMap{"region": "eu-west-1"}

	mergeLLMPatch(&input, existing, llmPatchAttributeKeys(body))

	// Metadata is the one deliberate exemption: UpdateLLM routes it through
	// mergeMetadataPreservingRedacted, which returns the stored value when the
	// incoming map is nil, so leaving it nil here is already correct.
	exempt := map[string]string{
		"metadata": "preserved by mergeMetadataPreservingRedacted when nil",
	}

	attrs := reflect.ValueOf(input.Data.Attributes)
	attrsType := attrs.Type()

	for i := 0; i < attrsType.NumField(); i++ {
		field := attrsType.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		if reason, ok := exempt[key]; ok {
			t.Logf("skipping %q: %s", key, reason)
			continue
		}

		value := attrs.Field(i)
		assert.False(t, value.IsZero(),
			"attribute %q was not restored by mergeLLMPatch, so a PATCH omitting it "+
				"would overwrite the stored value with a zero", key)
	}
}

func TestMergeLLMPatch_BudgetStartDateSurvivesAnUnrelatedPatch(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{"name":"Renamed"}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	existing := existingLLM()
	start := time.Date(2026, 3, 1, 12, 30, 0, 0, time.UTC)
	existing.BudgetStartDate = &start

	mergeLLMPatch(&input, existing, llmPatchAttributeKeys(body))

	assert.NotNil(t, input.Data.Attributes.BudgetStartDate)
	// Must round-trip through the format parseBudgetStartDate reads, or the
	// handler turns it back into nil and the data is lost anyway.
	assert.Equal(t, start, *parseBudgetStartDate(input.Data.Attributes.BudgetStartDate))
}

func TestMergeLLMPatch_BudgetStartDateCanStillBeCleared(t *testing.T) {
	body := []byte(`{"data":{"type":"llm","attributes":{"budget_start_date":null}}}`)

	var input LLMInput
	assert.NoError(t, json.Unmarshal(body, &input))

	existing := existingLLM()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	existing.BudgetStartDate = &start

	mergeLLMPatch(&input, existing, llmPatchAttributeKeys(body))

	assert.Nil(t, input.Data.Attributes.BudgetStartDate,
		"an explicitly supplied null must still clear the field")
}
