package api

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/stretchr/testify/assert"
)

// Compliance events were collected by the runner and then dropped when the
// handler built its response map, so a filter that raised them looked
// completely silent in the test panel -- the one place a filter author can see
// what their script actually did.
func TestFilterTestOutput_CarriesComplianceEvents(t *testing.T) {
	output := &scripting.ScriptOutput{
		Block:   false,
		Payload: "redacted body",
		ComplianceEvents: []scripting.ComplianceEventOutput{
			{
				EventType:   "pii_redacted",
				Severity:    "warning",
				Description: "Redacted 2 email addresses",
				Metadata:    map[string]interface{}{"count": 2},
			},
		},
	}

	outputMap := map[string]interface{}{
		"block":   output.Block,
		"payload": output.Payload,
		"message": output.Message,
	}
	if len(output.ComplianceEvents) > 0 {
		events := make([]map[string]interface{}, len(output.ComplianceEvents))
		for i, e := range output.ComplianceEvents {
			events[i] = map[string]interface{}{
				"event_type":  e.EventType,
				"severity":    e.Severity,
				"description": e.Description,
				"metadata":    e.Metadata,
			}
		}
		outputMap["compliance_events"] = events
	}

	body, err := json.Marshal(FilterTestOutput{Success: true, Output: outputMap})
	assert.NoError(t, err)

	var decoded struct {
		Output struct {
			ComplianceEvents []struct {
				EventType   string `json:"event_type"`
				Severity    string `json:"severity"`
				Description string `json:"description"`
			} `json:"compliance_events"`
		} `json:"output"`
	}
	assert.NoError(t, json.Unmarshal(body, &decoded))
	assert.Len(t, decoded.Output.ComplianceEvents, 1)
	assert.Equal(t, "pii_redacted", decoded.Output.ComplianceEvents[0].EventType)
	assert.Equal(t, "warning", decoded.Output.ComplianceEvents[0].Severity)
	assert.Equal(t, "Redacted 2 email addresses", decoded.Output.ComplianceEvents[0].Description)
}
