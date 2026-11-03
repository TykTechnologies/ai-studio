package services

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/rs/zerolog/log"
)

// Guardrail exceptions on the edge (features/GuardrailExceptions.md). The
// hub sends the approved exceptions of the Apps in the snapshot, each with
// the capability of its App; the edge holds them in memory (an edge always
// starts from a snapshot) and registers them as the guardrails' exception
// source, so they apply here exactly as on AI Studio: the same decide(), the
// same floor. The edge checks the floor again and drops an exception outside
// it (the hub never sends one), so the filter's own action applies. An
// exception retired or changed on the hub keeps applying here until the edge
// applies the next snapshot. A snapshot with none removes the source, so
// requests are screened exactly as before exceptions existed.

// edgeExceptionSource serves one applied snapshot's exceptions.
type edgeExceptionSource struct {
	byApp map[uint]*guardrails.AppExceptions
}

// AppExceptions implements scripting.ExceptionSource.
func (s *edgeExceptionSource) AppExceptions(_ context.Context, appID uint) *guardrails.AppExceptions {
	return s.byApp[appID]
}

// Hit implements scripting.ExceptionSource: the hit is reported to the hub
// in the next heartbeat.
func (s *edgeExceptionSource) Hit(id uint) {
	if id != 0 {
		GuardrailExceptionHits.add(uint32(id))
	}
}

// installGuardrailExceptions registers the snapshot's exceptions as the
// guardrails' exception source, or none when it has none.
func installGuardrailExceptions(cfgs []*pb.GuardrailExceptionConfig) {
	src := &edgeExceptionSource{byApp: map[uint]*guardrails.AppExceptions{}}
	for _, c := range cfgs {
		if c == nil || c.Id == 0 || c.AppId == 0 || c.FilterId == 0 {
			continue
		}
		capability, err := guardrails.ParseCapability(c.AppCapability)
		if err != nil {
			// Unknown to this edge: the strictest.
			capability = guardrails.CapabilityActing
		}
		ch := guardrails.Channel(c.Channel)
		if err := guardrails.CheckException(capability, ch, c.Action); err != nil {
			log.Error().Err(err).Uint32("app_id", c.AppId).Uint32("exception_id", c.Id).
				Msg("Guardrail exception from the hub is outside its App's floor; not used")
			continue
		}
		app := src.byApp[uint(c.AppId)]
		if app == nil {
			app = &guardrails.AppExceptions{Capability: capability}
			src.byApp[uint(c.AppId)] = app
		}
		app.Exceptions = append(app.Exceptions, guardrails.Exception{
			ID: uint(c.Id), FilterID: uint(c.FilterId), Detector: c.Detector,
			Channel: ch, ToolName: c.ToolName, Action: c.Action,
		})
	}
	if len(src.byApp) == 0 {
		scripting.SetExceptionSource(nil)
		return
	}
	scripting.SetExceptionSource(src)
}

// GuardrailExceptionHits holds the IDs of the guardrail exceptions that
// applied since the last heartbeat (grpc.TemplateMatchSource).
var GuardrailExceptionHits = &templateMatches{}
