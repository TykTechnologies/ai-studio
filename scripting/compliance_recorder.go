package scripting

import (
	"context"
	"encoding/json"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// RecordComplianceEvents converts script output compliance events to model events
// enriched with context from the filter execution site, then records them asynchronously.
func RecordComplianceEvents(ctx context.Context, output *ScriptOutput, filterName, filterScope string, appID, userID, llmID uint, vendor, modelName string) {
	if output == nil || len(output.ComplianceEvents) == 0 {
		return
	}

	now := time.Now()
	events := make([]*models.ComplianceEvent, 0, len(output.ComplianceEvents))
	for _, ce := range output.ComplianceEvents {
		metadataJSON := ""
		if ce.Metadata != nil {
			if b, err := json.Marshal(ce.Metadata); err == nil {
				metadataJSON = string(b)
			}
		}
		events = append(events, &models.ComplianceEvent{
			AppID:       appID,
			UserID:      userID,
			LLMID:       llmID,
			FilterName:  filterName,
			FilterScope: filterScope,
			EventType:   ce.EventType,
			Severity:    ce.Severity,
			Description: ce.Description,
			Metadata:    metadataJSON,
			Vendor:      vendor,
			ModelName:   modelName,
			TimeStamp:   now,
		})
	}

	analytics.RecordComplianceEvents(ctx, events)
}
