package openaiVendor

import (
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// normalizeServiceTier maps the service_tier an OpenAI reply reports to the
// tier the gateway records and prices by (models.IPriceAdjustedResponse):
// "default" (and "auto", should a reply echo the request) is list price, so
// it is recorded as no tier; "flex", "priority", "scale"... are kept.
func normalizeServiceTier(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "default" || t == "auto" {
		return ""
	}
	return t
}

// tieredResponse is a chat-completions reply billed at a service tier. The
// list price applies (multiplier 1) unless the LLM overrides the tier
// (LLM.PriceMultipliers).
type tieredResponse struct {
	models.ITokenResponse
	tier string
}

var _ models.IPriceAdjustedResponse = tieredResponse{}

func (t tieredResponse) ServiceTier() string      { return t.tier }
func (t tieredResponse) PriceMultiplier() float64 { return 1 }

// withServiceTier attaches the reply's service tier to r; a reply at list
// price is returned as is.
func withServiceTier(r models.ITokenResponse, raw string) models.ITokenResponse {
	if tier := normalizeServiceTier(raw); tier != "" && r != nil {
		return tieredResponse{ITokenResponse: r, tier: tier}
	}
	return r
}

// serviceTierProbe reads only the service_tier of a reply or stream chunk.
type serviceTierProbe struct {
	ServiceTier string `json:"service_tier"`
}
