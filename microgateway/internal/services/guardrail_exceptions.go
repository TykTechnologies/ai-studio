package services

import (
	"context"
	"sync/atomic"

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
			Channel: ch, ToolName: c.ToolName, Action: c.Action, AlertEvent: c.AlertEvent,
		})
	}
	if len(src.byApp) == 0 {
		installedExceptions.Store(nil)
		scripting.SetExceptionSource(nil)
		return
	}
	installedExceptions.Store(src)
	scripting.SetExceptionSource(src)
}

// installedExceptions is the exception source of the snapshot last applied.
var installedExceptions atomic.Pointer[edgeExceptionSource]

// InstalledGuardrailExceptions returns how many guardrail exceptions this
// edge holds for an App, from the snapshot it last applied. It exists for
// tests, which have no other way to see this in-memory configuration (the
// last-known-good tests assert it is rebuilt); nothing at run time calls it.
func InstalledGuardrailExceptions(appID uint) int {
	if src := installedExceptions.Load(); src != nil {
		if app := src.byApp[appID]; app != nil {
			return len(app.Exceptions)
		}
	}
	return 0
}

// GuardrailExceptionHits holds the IDs of the guardrail exceptions that
// applied since the last heartbeat (grpc.TemplateMatchSource).
var GuardrailExceptionHits = &templateMatches{}

// edgeCapabilitySource serves one applied snapshot's capability list
// (scripting.CapabilitySource), which counts in the checksum, so a
// capability attested, lowered or reset on the hub marks the edge pending.
type edgeCapabilitySource struct {
	byApp map[uint]guardrails.Capability
}

// AppCapability implements scripting.CapabilitySource.
func (s *edgeCapabilitySource) AppCapability(_ context.Context, appID uint) guardrails.Capability {
	return s.byApp[appID]
}

// installGuardrailCapabilities registers the snapshot's capabilities, or
// none (every app unattested). An unknown value is left out (acting).
func installGuardrailCapabilities(cfgs []*pb.AppGuardrailCapabilityConfig) {
	src := &edgeCapabilitySource{byApp: map[uint]guardrails.Capability{}}
	for _, c := range cfgs {
		if c == nil || c.AppId == 0 {
			continue
		}
		capability, err := guardrails.ParseCapability(c.Capability)
		if err != nil || capability == guardrails.CapabilityUnattested {
			continue
		}
		src.byApp[uint(c.AppId)] = capability
	}
	if len(src.byApp) == 0 {
		scripting.SetCapabilitySource(nil)
		return
	}
	scripting.SetCapabilitySource(src)
}
