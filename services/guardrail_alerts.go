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
	// Name is the alert's name too, under the key the webhook presets show
	// for an object.
	Name              string   `json:"name"`
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

// GuardrailAlertEvent is the bus event a guardrail alert is raised as:
// shaped like an object event (ObjectEventPayload), with the alert as its
// object (the compliance event is its id), so the webhook presets render it
// as they render any event.
type GuardrailAlertEvent struct {
	ObjectType string                `json:"object_type"`
	Action     string                `json:"action"`
	ObjectID   uint                  `json:"object_id"`
	Timestamp  string                `json:"timestamp"`
	Object     GuardrailAlertPayload `json:"object"`
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

// alertBatch memoises, for one batch of stored events, what validating
// their alerts reads: each filter (and its parsed config), each app's name
// and the exceptions naming each alert, so a batch costs one lookup per
// distinct filter, app and (app, filter, alert) rather than per event.
type alertBatch struct {
	s          *Service
	filters    map[uint]*alertFilterEntry
	apps       map[uint]string
	exceptions map[alertExceptionKey][]uint
}

type alertFilterEntry struct {
	filter *models.Filter
	cfg    guardrails.Config
	ok     bool
}

type alertExceptionKey struct {
	appID, filterID uint
	name            string
}

// filter returns the guardrail filter id with its parsed config; ok is
// false when there is none or its config does not parse.
func (b *alertBatch) filter(id uint) *alertFilterEntry {
	if e, ok := b.filters[id]; ok {
		return e
	}
	e := &alertFilterEntry{filter: b.s.alertFilter(id)}
	if e.filter != nil {
		cfg, err := guardrails.ParseConfig(e.filter.Config)
		e.cfg, e.ok = cfg, err == nil
	}
	b.filters[id] = e
	return e
}

func (b *alertBatch) appName(id uint) string {
	if name, ok := b.apps[id]; ok {
		return name
	}
	var app models.App
	name := ""
	if b.s.DB.Select("id", "name").First(&app, id).Error == nil {
		name = app.Name
	}
	b.apps[id] = name
	return name
}

// exceptionIDs are the approved exceptions of the app and filter whose
// alert event is name, by id.
func (b *alertBatch) exceptionIDs(appID, filterID uint, name string) []uint {
	key := alertExceptionKey{appID, filterID, name}
	if ids, ok := b.exceptions[key]; ok {
		return ids
	}
	var ids []uint
	if err := b.s.DB.Model(&models.GuardrailException{}).
		Where("app_id = ? AND filter_id = ? AND alert_event = ? AND status = ?", appID, filterID, name, models.GuardrailExceptionApproved).
		Order("id").Pluck("id", &ids).Error; err != nil {
		ids = nil
	}
	b.exceptions[key] = ids
	return ids
}

func (s *Service) publishGuardrailAlerts(events []*models.ComplianceEvent) {
	batch := &alertBatch{s: s, filters: map[uint]*alertFilterEntry{}, apps: map[uint]string{}, exceptions: map[alertExceptionKey][]uint{}}
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
		fe := batch.filter(meta.FilterID)
		if fe.filter == nil || fe.filter.Name != ev.FilterName {
			slog.Warn("guardrail alert ignored: its event does not come from the guardrail filter it names", "compliance_event_id", ev.ID, "filter_id", meta.FilterID)
			continue
		}
		if !fe.ok {
			continue
		}
		filter, cfg := fe.filter, fe.cfg
		for _, name := range names {
			payload, ok := batch.alertPayload(ev, meta, name, filter, cfg)
			if !ok {
				slog.Warn("guardrail alert ignored: no alert of that name is configured on the event's filter or exception", "compliance_event_id", ev.ID, "filter_id", filter.ID, "alert", safeAlertName(name))
				continue
			}
			if ev.AppID != 0 {
				payload.AppName = batch.appName(ev.AppID)
			}
			payload.Name = payload.Alert
			if err := s.SystemEvents.PublishLocal(payload.Topic, GuardrailAlertEvent{
				ObjectType: "guardrail_alert", Action: "raised", ObjectID: ev.ID,
				Timestamp: payload.Timestamp, Object: payload,
			}); err != nil {
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
func (b *alertBatch) alertPayload(ev *models.ComplianceEvent, meta alertEventMetadata, name string, filter *models.Filter, cfg guardrails.Config) (GuardrailAlertPayload, bool) {
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
	// The lowest-id approved exception of this app and filter naming the
	// alert that the event lists.
	listed := map[uint]bool{}
	for _, id := range meta.Exceptions {
		listed[id] = true
	}
	for _, id := range b.exceptionIDs(ev.AppID, filter.ID, name) {
		if listed[id] {
			// An exception's alert never carries the flagged text.
			p.ExceptionID = id
			return p, true
		}
	}
	return p, false
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
