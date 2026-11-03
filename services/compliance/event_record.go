package compliance

import (
	"encoding/json"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ErrEventNotFound is returned by EventRecord for an id with no compliance
// event.
var ErrEventNotFound = errors.New("compliance event not found")

// EventRecord returns one compliance event as the events list returns it
// (GET /compliance/events/:id), so a link to an event (a guardrail alert's or
// outcome event's url) opens it whatever the Compliance page's period. It is
// an Enterprise feature like the list: ErrEnterpriseFeature without it.
func EventRecord(db *gorm.DB, id uint) (*ComplianceEventRecord, error) {
	if !IsEnterpriseAvailable() {
		return nil, ErrEnterpriseFeature
	}
	var e models.ComplianceEvent
	if err := db.First(&e, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	var metadata map[string]interface{}
	if e.Metadata != "" {
		_ = json.Unmarshal([]byte(e.Metadata), &metadata)
	}
	rec := &ComplianceEventRecord{
		ID: e.ID, AppID: e.AppID, UserID: e.UserID, LLMID: e.LLMID,
		FilterName: e.FilterName, FilterScope: e.FilterScope, EventType: e.EventType,
		Severity: e.Severity, Description: e.Description, Metadata: metadata,
		Vendor: e.Vendor, ModelName: e.ModelName, Timestamp: e.TimeStamp,
	}
	if e.AppID > 0 {
		var app models.App
		if db.Select("id", "name").First(&app, e.AppID).Error == nil {
			rec.AppName = app.Name
		}
	}
	return rec, nil
}
