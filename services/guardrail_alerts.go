package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
)

// Guardrail alerts (the "Log and raise event" action, features/Filters.md
// and features/GuardrailExceptions.md). A guardrail filter's compliance
// event names the alert its finding raised (metadata.alert, and alerts when
// there are several). The hub raises one bus event per alerting compliance
// event and name, on guardrail.alert.<name> (DirLocal), once the event is
// stored: for events recorded here and for those edges send in their
// analytics pulse, so edge alerts arrive with the pulse interval's delay.
// Webhooks subscribe to the topic (Enterprise).
//
// The event's metadata is not trusted: script filters cannot set the keys
// (scripting.ReservedEventKeys), but an edge is semi-trusted. A name is
// raised only when the hub's own configuration names it: the alert of the
// event's filter (matched by ID and name, a guardrail filter), or the
// alert_event of an exception of the event's app and filter that the event
// says softened it. Anything else is logged and dropped.

// GuardrailAlertPayload is what a guardrail alert carries. It never carries
// the flagged text, except masked excerpts when the filter's alert opts in
// (include_excerpt, which needs evidence.excerpt); an exception's alert
// never does.
type GuardrailAlertPayload struct {
	Alert             string   `json:"alert"`
	Topic             string   `json:"topic"`
	FilterID          uint     `json:"filter_id"`
	FilterName        string   `json:"filter_name"`
	AppID             uint     `json:"app_id"`
	AppName           string   `json:"app_name,omitempty"`
	Detector          string   `json:"detector,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	Roles             []string `json:"roles,omitempty"`
	ToolNames         []string `json:"tool_names,omitempty"`
	MaxScore          float64  `json:"max_score"`
	Action            string   `json:"action"`
	ExceptionID       uint     `json:"exception_id,omitempty"`
	RequestID         string   `json:"request_id,omitempty"`
	ComplianceEventID uint     `json:"compliance_event_id"`
	Timestamp         string   `json:"timestamp"`
	Link              string   `json:"link"`
	Excerpts          []string `json:"excerpts,omitempty"`
}

// alertEventMetadata is the part of a guardrail event's metadata an alert
// reads.
type alertEventMetadata struct {
	Alert      string   `json:"alert"`
	Alerts     []string `json:"alerts"`
	FilterID   uint     `json:"filter_id"`
	Detector   string   `json:"detector"`
	Kinds      []string `json:"kinds"`
	Roles      []string `json:"roles"`
	ToolNames  []string `json:"tool_names"`
	MaxScore   float64  `json:"max_score"`
	Action     string   `json:"action"`
	Exceptions []uint   `json:"exceptions"`
	RequestID  string   `json:"request_id"`
	Segments   []struct {
		Excerpt string `json:"excerpt"`
	} `json:"segments"`
}

func (m alertEventMetadata) names() []string {
	var out []string
	if m.Alert != "" {
		out = append(out, m.Alert)
	}
	for _, n := range m.Alerts {
		if n != "" && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// PublishGuardrailAlerts raises the alerts of stored compliance events
// (analytics.SetComplianceEventsStored). It never panics into the caller,
// the analytics writer.
func (s *Service) PublishGuardrailAlerts(events []*models.ComplianceEvent) {
	if s.SystemEvents == nil {
		return
	}
	safe.Call("guardrail_alerts", func() { s.publishGuardrailAlerts(events) })
}

func (s *Service) publishGuardrailAlerts(events []*models.ComplianceEvent) {
	filters := map[uint]*models.Filter{}
	apps := map[uint]string{}
	for _, ev := range events {
		if ev == nil || !strings.HasPrefix(ev.EventType, "guardrail.") || !strings.Contains(ev.Metadata, `"alert`) {
			continue
		}
		var meta alertEventMetadata
		if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil {
			continue
		}
		names := meta.names()
		if len(names) == 0 {
			continue
		}
		filter, ok := filters[meta.FilterID]
		if !ok {
			filter = s.alertFilter(meta.FilterID)
			filters[meta.FilterID] = filter
		}
		if filter == nil || filter.Name != ev.FilterName {
			slog.Warn("guardrail alert ignored: its event does not come from the guardrail filter it names", "compliance_event_id", ev.ID, "filter_id", meta.FilterID)
			continue
		}
		cfg, err := guardrails.ParseConfig(filter.Config)
		if err != nil {
			continue
		}
		for _, name := range names {
			payload, ok := s.alertPayload(ev, meta, name, filter, cfg)
			if !ok {
				slog.Warn("guardrail alert ignored: no alert of that name is configured on the event's filter or exception", "compliance_event_id", ev.ID, "filter_id", filter.ID, "alert", safeAlertName(name))
				continue
			}
			if _, seen := apps[ev.AppID]; !seen && ev.AppID != 0 {
				var app models.App
				if s.DB.Select("id", "name").First(&app, ev.AppID).Error == nil {
					apps[ev.AppID] = app.Name
				} else {
					apps[ev.AppID] = ""
				}
			}
			payload.AppName = apps[ev.AppID]
			if err := s.SystemEvents.PublishLocal(payload.Topic, payload); err != nil {
				slog.Warn("could not raise a guardrail alert", "topic", payload.Topic, "error", err)
			}
		}
	}
}

// alertFilter loads a guardrail filter by ID; nil when there is none.
func (s *Service) alertFilter(id uint) *models.Filter {
	if id == 0 {
		return nil
	}
	var f models.Filter
	if err := s.DB.Select("id", "name", "kind", "config").First(&f, id).Error; err != nil || !f.IsGuardrail() {
		return nil
	}
	return &f
}

// alertPayload builds the alert name raises for ev, if the hub's own
// configuration names it: the filter's alert, else an exception of the
// event's app and filter that the event lists.
func (s *Service) alertPayload(ev *models.ComplianceEvent, meta alertEventMetadata, name string, filter *models.Filter, cfg guardrails.Config) (GuardrailAlertPayload, bool) {
	if !guardrails.ValidAlertEvent(name) {
		return GuardrailAlertPayload{}, false
	}
	p := GuardrailAlertPayload{
		Alert: name, Topic: guardrails.AlertTopicPrefix + name,
		FilterID: filter.ID, FilterName: filter.Name, AppID: ev.AppID,
		Detector: meta.Detector, Kinds: meta.Kinds, Roles: meta.Roles, ToolNames: meta.ToolNames,
		MaxScore: meta.MaxScore, Action: meta.Action, RequestID: meta.RequestID,
		ComplianceEventID: ev.ID, Timestamp: ev.TimeStamp.UTC().Format(time.RFC3339),
		Link: fmt.Sprintf("/admin/compliance?event=%d", ev.ID),
	}
	for _, a := range []*guardrails.Alert{cfg.Alert, cfg.ToolResults.Alert, cfg.ToolCalls.Alert, cfg.ToolDefinitions.Alert} {
		if a == nil || a.Event != name {
			continue
		}
		if a.IncludeExcerpt && cfg.Evidence.Excerpt {
			for _, seg := range meta.Segments {
				if seg.Excerpt != "" {
					p.Excerpts = append(p.Excerpts, seg.Excerpt)
				}
			}
		}
		return p, true
	}
	if len(meta.Exceptions) == 0 || ev.AppID == 0 {
		return p, false
	}
	var e models.GuardrailException
	if err := s.DB.Select("id").Where("id IN ? AND app_id = ? AND filter_id = ? AND alert_event = ? AND status = ?", meta.Exceptions, ev.AppID, filter.ID, name, models.GuardrailExceptionApproved).
		First(&e).Error; err != nil {
		return p, false
	}
	// An exception's alert never carries the flagged text.
	p.ExceptionID = e.ID
	return p, true
}

// safeAlertName bounds an untrusted alert name for a log line.
func safeAlertName(name string) string {
	if len(name) > 64 {
		name = name[:64]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, name)
}
