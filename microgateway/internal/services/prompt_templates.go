package services

import (
	"context"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/rs/zerolog/log"
)

// Trusted prompt templates on the edge (features/TrustedPromptTemplates.md).
// The hub sends the approved templates of the Apps in the snapshot; the edge
// holds them in memory (an edge always starts from a snapshot) and registers
// them as the guardrails' template source, so matching runs here exactly as
// on AI Studio: the same matcher, the same scan plan. A template retired,
// deleted or changed on the hub keeps matching here until the edge applies
// the next snapshot. A snapshot with none removes the source, so requests
// are screened exactly as before templates existed.

// edgeTemplateSource serves one applied snapshot's templates.
type edgeTemplateSource struct {
	byApp map[uint][]*templates.Template
}

// ApprovedTemplates implements scripting.PromptTemplateSource.
func (s *edgeTemplateSource) ApprovedTemplates(_ context.Context, appID uint) []*templates.Template {
	return s.byApp[appID]
}

// Matched implements scripting.PromptTemplateSource: the match is reported
// to the hub in the next heartbeat.
func (s *edgeTemplateSource) Matched(t *templates.Template) {
	if t != nil && t.RecordID != 0 {
		PromptTemplateMatches.add(uint32(t.RecordID))
	}
}

// installPromptTemplates registers the snapshot's templates as the
// guardrails' template source, or none when it has none. A template the
// edge finds invalid (it never should: the hub validated it) is left out
// and logged: its App's requests are then scored whole.
func installPromptTemplates(cfgs []*pb.PromptTemplateConfig) {
	if len(cfgs) == 0 {
		scripting.SetPromptTemplateSource(nil)
		return
	}
	src := &edgeTemplateSource{byApp: map[uint][]*templates.Template{}}
	for _, c := range cfgs {
		if c == nil {
			continue
		}
		t := &templates.Template{
			RecordID: uint(c.Id), Name: c.Name, Version: int(c.Version),
			Placement: templates.Placement{Role: c.PlacementRole, Message: c.PlacementMessage, Anchor: c.PlacementAnchor}.Normalize(),
		}
		for _, p := range c.Parts {
			t.Parts = append(t.Parts, templates.Part{Const: p.GetConstant(), Slot: p.GetSlot()})
		}
		if err := templates.Validate(t.Placement, t.Parts); err != nil {
			log.Error().Err(err).Uint32("app_id", c.AppId).Str("template", t.Ref()).Msg("Prompt template from the hub is invalid; not used")
			continue
		}
		src.byApp[uint(c.AppId)] = append(src.byApp[uint(c.AppId)], t)
	}
	if len(src.byApp) == 0 {
		scripting.SetPromptTemplateSource(nil)
		return
	}
	scripting.SetPromptTemplateSource(src)
}

// PromptTemplateMatches holds the IDs of the templates that matched since
// the last heartbeat (grpc.TemplateMatchSource).
var PromptTemplateMatches = &templateMatches{}

// maxPendingTemplateMatches bounds the IDs held between heartbeats; an App
// has at most templates.MaxApprovedPerApp approved, so this is many Apps.
const maxPendingTemplateMatches = 4096

type templateMatches struct {
	mu  sync.Mutex
	ids map[uint32]struct{}
}

func (m *templateMatches) add(id uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ids == nil {
		m.ids = map[uint32]struct{}{}
	}
	if len(m.ids) < maxPendingTemplateMatches {
		m.ids[id] = struct{}{}
	}
}

// DrainMatched returns the IDs matched since the last call.
func (m *templateMatches) DrainMatched() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ids) == 0 {
		return nil
	}
	out := make([]uint32, 0, len(m.ids))
	for id := range m.ids {
		out = append(out, id)
	}
	m.ids = nil
	return out
}

// Requeue puts back IDs a heartbeat failed to deliver.
func (m *templateMatches) Requeue(ids []uint32) {
	for _, id := range ids {
		m.add(id)
	}
}
