package api

import (
	"encoding/json"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// mergeLLMPatch fills in the fields a PATCH body did not mention, from the LLM
// as it currently stands.
//
// PATCH bound straight onto LLMInput, whose attributes are plain values rather
// than pointers, so an omitted field arrived as its zero value and was written
// as such. Sending {"filters":[3]} therefore wiped the provider: name, vendor,
// endpoint, default model and API key written back empty, and active flipped
// to false. Nothing in the API reference said the verb behaved this way.
//
// `present` is the set of attribute keys that actually appeared in the request
// body, so an explicit null or "" can still clear a field while an absent key
// leaves it alone.
func mergeLLMPatch(input *LLMInput, existing *models.LLM, present map[string]json.RawMessage) {
	attrs := &input.Data.Attributes

	has := func(key string) bool {
		_, ok := present[key]
		return ok
	}

	if !has("name") {
		attrs.Name = existing.Name
	}
	if !has("api_key") {
		// GetLLMByID hands back the $SECRET/ reference or the raw key; either
		// is the correct thing to write straight back.
		attrs.APIKey = existing.APIKey
	}
	if !has("api_endpoint") {
		attrs.APIEndpoint = existing.APIEndpoint
	}
	if !has("privacy_score") {
		attrs.PrivacyScore = existing.PrivacyScore
	}
	if !has("short_description") {
		attrs.ShortDescription = existing.ShortDescription
	}
	if !has("long_description") {
		attrs.LongDescription = existing.LongDescription
	}
	if !has("logo_url") {
		attrs.LogoURL = existing.LogoURL
	}
	if !has("vendor") {
		attrs.Vendor = string(existing.Vendor)
	}
	if !has("active") {
		attrs.Active = existing.Active
	}
	if !has("default_model") {
		attrs.DefaultModel = existing.DefaultModel
	}
	if !has("allowed_models") {
		attrs.AllowedModels = existing.AllowedModels
	}
	if !has("namespace") {
		attrs.Namespace = existing.Namespace
	}
	if !has("dont_log_bodies") {
		attrs.DontLogBodies = existing.DontLogBodies
	}
	if !has("monthly_budget") {
		attrs.MonthlyBudget = existing.MonthlyBudget
	}
	if !has("filters") {
		attrs.Filters = make([]uint, 0, len(existing.Filters))
		for _, f := range existing.Filters {
			attrs.Filters = append(attrs.Filters, f.ID)
		}
	}
}

// llmPatchAttributeKeys reports which attribute keys a request body actually
// carried, so absent keys can be distinguished from explicitly zeroed ones.
func llmPatchAttributeKeys(body []byte) map[string]json.RawMessage {
	var probe struct {
		Data struct {
			Attributes map[string]json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil
	}
	return probe.Data.Attributes
}

// redactedAPIKeyRetained reports whether the incoming api_key is the redaction
// placeholder the API hands out, which must never be written back as a
// credential. UpdateLLM already guards this; kept here so the intent is
// visible alongside the merge.
func redactedAPIKeyRetained(apiKey string) bool {
	return apiKey == services.REDACTED_VALUE
}
