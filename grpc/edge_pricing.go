package grpc

import (
	"math"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// maxEdgeRequestedModel bounds the requested model name an edge reports.
const maxEdgeRequestedModel = 256

// edgeRequestedModel is the requested model an edge reported for a chat
// record, or "" when it is implausibly long (edge values are untrusted).
func edgeRequestedModel(s string) string {
	if len(s) > maxEdgeRequestedModel {
		return ""
	}
	return s
}

// edgePriceMultiplier is the price multiplier an edge reported for a chat
// record, or nil when it reported none (older edges) or one outside what an
// LLM may be configured with (0 to models.MaxPriceMultiplier). A nil
// multiplier makes a recalculation fall back to the row's cost ratio.
func edgePriceMultiplier(m *float64) *float64 {
	if m == nil || math.IsNaN(*m) || *m < 0 || *m > models.MaxPriceMultiplier {
		return nil
	}
	v := *m
	return &v
}
