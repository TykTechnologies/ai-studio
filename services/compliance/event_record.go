package compliance

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
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
	rec := toRecord(&e)
	if e.AppID > 0 {
		rec.AppName = appName(db, e.AppID)
	}
	return &rec, nil
}

func toRecord(e *models.ComplianceEvent) ComplianceEventRecord {
	var metadata map[string]interface{}
	if e.Metadata != "" {
		_ = json.Unmarshal([]byte(e.Metadata), &metadata)
	}
	return ComplianceEventRecord{
		ID: e.ID, AppID: e.AppID, UserID: e.UserID, LLMID: e.LLMID,
		FilterName: e.FilterName, FilterScope: e.FilterScope, EventType: e.EventType,
		Severity: e.Severity, Description: e.Description, Metadata: metadata,
		Vendor: e.Vendor, ModelName: e.ModelName, Timestamp: e.TimeStamp,
	}
}

func appName(db *gorm.DB, id uint) string {
	var app models.App
	if db.Select("id", "name").First(&app, id).Error == nil {
		return app.Name
	}
	return ""
}

// ErrInvalidRequestID is a request id that is not the gateway's
// (pkg/requestid.Valid).
var ErrInvalidRequestID = errors.New("request_id is not a gateway request id")

// RequestEventWindow is how far either side of the given time
// EventsByRequest looks: a request's events are recorded within seconds of
// it, an edge's within a heartbeat or two.
const RequestEventWindow = 10 * time.Minute

// RequestEventLimit caps EventsByRequest: one request has a handful of
// compliance events (one per filter finding).
const RequestEventLimit = 20

// EventsByRequest returns the App's compliance events that record requestID
// as metadata.request_id, recorded within RequestEventWindow of at, oldest
// first, at most RequestEventLimit (tkg.2: an agent trace's guardrail outcome
// links to them). The App and the window bound the scan to the
// idx_ce_app_time index; the id is validated as hex before it reaches the
// LIKE, so it needs no escaping. Enterprise only, like the list.
func EventsByRequest(db *gorm.DB, appID uint, requestID string, at time.Time) ([]ComplianceEventRecord, error) {
	if !IsEnterpriseAvailable() {
		return nil, ErrEnterpriseFeature
	}
	if !requestid.Valid(requestID) {
		return nil, ErrInvalidRequestID
	}
	var events []models.ComplianceEvent
	if err := db.Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", appID, at.Add(-RequestEventWindow), at.Add(RequestEventWindow)).
		Where("metadata LIKE ?", `%"request_id":"`+requestID+`"%`).
		Order("time_stamp ASC").Order("id ASC").Limit(RequestEventLimit).Find(&events).Error; err != nil {
		return nil, err
	}
	out := make([]ComplianceEventRecord, 0, len(events))
	name := ""
	if len(events) > 0 {
		name = appName(db, appID)
	}
	for i := range events {
		rec := toRecord(&events[i])
		rec.AppName = name
		out = append(out, rec)
	}
	return out, nil
}
