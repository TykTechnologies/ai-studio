package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
)

// Guardrail outcome events (features/Filters.md, "Outcome events"). Where an
// alert is raised only by the alert action, an outcome event is raised for
// what a guardrail filter did to traffic, with no configuration: the hub
// raises one bus event (DirLocal) per stored guardrail compliance event that
// records a block, a quarantine or a softening by the LLM judge, on
//
//	guardrail.outcome.blocked         the request or response was blocked
//	guardrail.outcome.quarantined     content was quarantined
//	guardrail.outcome.judge_softened  the judge's benign opinion softened a
//	                                  block or quarantine (judge mode soften)
//
// Webhooks and Studio plugins subscribe to them. Like alerts they are raised
// once the event is stored, so for edge traffic they follow the analytics
// pulse, and only Studio raises them (eventbridge.IsGuardrailTopic): the
// compliance event must name the guardrail filter it comes from (filter_id
// and name, a key script filters cannot set), and a judge softening counts
// only when that filter's judge is on soften. A shadow filter's events
// change no traffic and raise none. The payload never carries the flagged
// text.

// Guardrail outcome event names (GuardrailOutcomePayload.Outcome), each
// raised on eventbridge.GuardrailOutcomeTopicPrefix + name.
const (
	GuardrailOutcomeBlocked       = "blocked"
	GuardrailOutcomeQuarantined   = "quarantined"
	GuardrailOutcomeJudgeSoftened = "judge_softened"
)

// GuardrailOutcomeTopics are the outcome event topics, sorted.
var GuardrailOutcomeTopics = []string{
	eventbridge.GuardrailOutcomeTopicPrefix + GuardrailOutcomeBlocked,
	eventbridge.GuardrailOutcomeTopicPrefix + GuardrailOutcomeJudgeSoftened,
	eventbridge.GuardrailOutcomeTopicPrefix + GuardrailOutcomeQuarantined,
}

// GuardrailOutcomePayload is what a guardrail outcome event carries.
type GuardrailOutcomePayload struct {
	// Name summarises the event (filter, detector, app) under the key the
	// webhook presets show for an object.
	Name              string   `json:"name"`
	Outcome           string   `json:"outcome"`
	Topic             string   `json:"topic"`
	FilterID          uint     `json:"filter_id"`
	FilterName        string   `json:"filter_name"`
	AppID             uint     `json:"app_id"`
	AppName           string   `json:"app_name,omitempty"`
	EventType         string   `json:"event_type"`
	Severity          string   `json:"severity,omitempty"`
	Detector          string   `json:"detector,omitempty"`
	Detectors         []string `json:"detectors,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	Roles             []string `json:"roles,omitempty"`
	ToolNames         []string `json:"tool_names,omitempty"`
	MaxScore          float64  `json:"max_score,omitempty"`
	Action            string   `json:"action"`
	ReasonCode        string   `json:"reason_code,omitempty"`
	ExceptionIDs      []uint   `json:"exception_ids,omitempty"`
	RequestID         string   `json:"request_id,omitempty"`
	ComplianceEventID uint     `json:"compliance_event_id"`
	Timestamp         string   `json:"timestamp"`
	Link              string   `json:"link"`
	// URL is Link made absolute with SITE_URL (see GuardrailAlertPayload).
	URL string `json:"url,omitempty"`
	// Judge is the judge's opinion, on judge_softened only.
	Judge *GuardrailOutcomeJudge `json:"judge,omitempty"`
}

// GuardrailOutcomeJudge is the LLM judge's opinion that softened a verdict.
type GuardrailOutcomeJudge struct {
	LLMID         uint   `json:"llm_id,omitempty"`
	Model         string `json:"model,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	Verdict       string `json:"verdict"`
	Reason        string `json:"reason,omitempty"`
	Coverage      string `json:"coverage,omitempty"`
	SoftenedFrom  string `json:"softened_from"`
	SoftenedTo    string `json:"softened_to"`
	Cached        bool   `json:"cached,omitempty"`
}

// GuardrailOutcomeEvent is the bus event an outcome is raised as, shaped
// like an object event (ObjectEventPayload) as alerts are, so the webhook
// presets render it: object type "guardrail", the outcome as its action.
type GuardrailOutcomeEvent struct {
	ObjectType string                  `json:"object_type"`
	Action     string                  `json:"action"`
	ObjectID   uint                    `json:"object_id"`
	Timestamp  string                  `json:"timestamp"`
	Object     GuardrailOutcomePayload `json:"object"`
}

// outcomeEventMetadata is the part of a guardrail event's metadata an
// outcome reads.
type outcomeEventMetadata struct {
	FilterID   uint     `json:"filter_id"`
	Shadow     bool     `json:"shadow"`
	Detector   string   `json:"detector"`
	Kinds      []string `json:"kinds"`
	Roles      []string `json:"roles"`
	ToolNames  []string `json:"tool_names"`
	MaxScore   float64  `json:"max_score"`
	Action     string   `json:"action"`
	Blocked    bool     `json:"blocked"`
	ReasonCode string   `json:"reason_code"`
	Exceptions []uint   `json:"exceptions"`
	RequestID  string   `json:"request_id"`
	Judge      *struct {
		LLMID         uint     `json:"llm_id"`
		Model         string   `json:"model"`
		Mode          string   `json:"mode"`
		PromptVersion string   `json:"prompt_version"`
		Detectors     []string `json:"detectors"`
		Verdict       string   `json:"verdict"`
		Reason        string   `json:"reason"`
		Coverage      string   `json:"coverage"`
		SoftenedFrom  string   `json:"softened_from"`
		SoftenedTo    string   `json:"softened_to"`
		Cached        bool     `json:"cached"`
	} `json:"judge"`
}

// judgeEventType is scripting.JudgeEventType (services cannot import
// scripting).
const judgeEventType = "guardrail.judge"

// outcome is the outcome ev records, "" when none: a judge softening on the
// judge's own event, else a block or quarantine.
func (m outcomeEventMetadata) outcome(eventType string) string {
	if eventType == judgeEventType {
		if m.Judge != nil && m.Judge.Mode == guardrails.JudgeSoften && m.Judge.SoftenedTo != "" {
			return GuardrailOutcomeJudgeSoftened
		}
		return ""
	}
	switch {
	case m.Action == guardrails.ActionBlock || m.Blocked:
		return GuardrailOutcomeBlocked
	case m.Action == guardrails.ActionQuarantine:
		return GuardrailOutcomeQuarantined
	}
	return ""
}

// PublishGuardrailEvents raises the alerts and outcome events of stored
// compliance events (analytics.SetComplianceEventsStored). It never panics
// into the caller, the analytics writer.
func (s *Service) PublishGuardrailEvents(events []*models.ComplianceEvent) {
	s.PublishGuardrailAlerts(events)
	s.PublishGuardrailOutcomes(events)
}

// PublishGuardrailOutcomes raises the outcome events of stored compliance
// events. It never panics into the caller.
func (s *Service) PublishGuardrailOutcomes(events []*models.ComplianceEvent) {
	if s.SystemEvents == nil {
		return
	}
	safe.Call("guardrail_outcomes", func() { s.publishGuardrailOutcomes(events) })
}

func (s *Service) publishGuardrailOutcomes(events []*models.ComplianceEvent) {
	batch := &alertBatch{s: s, filters: map[uint]*alertFilterEntry{}, apps: map[uint]string{}, exceptions: map[alertExceptionKey][]uint{}}
	for _, ev := range events {
		if ev == nil || !strings.HasPrefix(ev.EventType, "guardrail.") {
			continue
		}
		var meta outcomeEventMetadata
		if err := json.Unmarshal([]byte(ev.Metadata), &meta); err != nil || meta.Shadow {
			continue
		}
		outcome := meta.outcome(ev.EventType)
		if outcome == "" {
			continue
		}
		fe := batch.filter(meta.FilterID)
		if fe.filter == nil || fe.filter.Name != ev.FilterName {
			slog.Warn("guardrail outcome ignored: its event does not come from the guardrail filter it names", "compliance_event_id", ev.ID, "filter_id", meta.FilterID)
			continue
		}
		if outcome == GuardrailOutcomeJudgeSoftened && (!fe.ok || fe.cfg.Judge.Mode != guardrails.JudgeSoften || fe.cfg.Shadow) {
			slog.Warn("guardrail outcome ignored: a judge softening from a filter whose judge does not soften", "compliance_event_id", ev.ID, "filter_id", fe.filter.ID)
			continue
		}
		p := outcomePayload(ev, meta, outcome, fe.filter)
		if ev.AppID != 0 {
			p.AppName = batch.appName(ev.AppID)
		}
		p.Name = outcomeName(p)
		if err := s.SystemEvents.PublishLocal(p.Topic, GuardrailOutcomeEvent{
			ObjectType: "guardrail", Action: outcome, ObjectID: ev.ID,
			Timestamp: p.Timestamp, Object: p,
		}); err != nil {
			slog.Warn("could not raise a guardrail outcome event", "topic", p.Topic, "error", err)
		}
	}
}

func outcomePayload(ev *models.ComplianceEvent, meta outcomeEventMetadata, outcome string, filter *models.Filter) GuardrailOutcomePayload {
	p := GuardrailOutcomePayload{
		Outcome: outcome, Topic: eventbridge.GuardrailOutcomeTopicPrefix + outcome,
		FilterID: filter.ID, FilterName: filter.Name, AppID: ev.AppID,
		EventType: ev.EventType, Severity: ev.Severity,
		Detector: meta.Detector, Kinds: meta.Kinds, Roles: meta.Roles, ToolNames: meta.ToolNames,
		MaxScore: meta.MaxScore, Action: meta.Action, ReasonCode: meta.ReasonCode,
		ExceptionIDs: meta.Exceptions, RequestID: meta.RequestID,
		ComplianceEventID: ev.ID, Timestamp: ev.TimeStamp.UTC().Format(time.RFC3339),
		Link: complianceEventLink(ev.ID), URL: complianceEventURL(ev.ID),
	}
	if p.Action == "" && meta.Blocked {
		p.Action = guardrails.ActionBlock
	}
	if outcome == GuardrailOutcomeJudgeSoftened {
		j := meta.Judge
		p.Action = j.SoftenedTo
		p.Detectors = j.Detectors
		p.Judge = &GuardrailOutcomeJudge{
			LLMID: j.LLMID, Model: j.Model, PromptVersion: j.PromptVersion,
			Verdict: j.Verdict, Reason: j.Reason, Coverage: j.Coverage,
			SoftenedFrom: j.SoftenedFrom, SoftenedTo: j.SoftenedTo, Cached: j.Cached,
		}
	}
	return p
}

// outcomeName is the one-line summary the webhook presets show.
func outcomeName(p GuardrailOutcomePayload) string {
	what := p.Detector
	if what == "" && len(p.Detectors) > 0 {
		what = strings.Join(p.Detectors, ", ")
	}
	if what == "" {
		what = strings.TrimPrefix(p.EventType, "guardrail.")
	}
	name := p.FilterName + ": " + what
	if p.AppName != "" {
		name += " (app " + p.AppName + ")"
	} else if p.AppID != 0 {
		name += fmt.Sprintf(" (app %d)", p.AppID)
	}
	return name
}
