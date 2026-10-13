package conformance

// Adversarial tests: the conformance suite must fail providers broken in
// ways its own cases do not name, and must report rather than crash or hang
// on a provider that panics or ignores its context.

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

func TestAdversarial_ConformanceCatchesMoreBrokenProviders(t *testing.T) {
	cases := map[string]*fake{
		"ignores tool results it is sent as role tool": {caps: guardrails.Capabilities{ToolSegments: true}, userOnly: true},
		"rewrites by position":                         {caps: guardrails.Capabilities{Redacts: true, ToolSegments: true}, rewriteByPosition: true},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if failures := Check(build(f), opts); len(failures) == 0 {
				t.Error("the conformance suite passed a broken provider")
			}
		})
	}
}

// A provider that panics on a tool segment is a failure the suite reports,
// not a crash of the test binary.
func TestAdversarial_ConformanceSurvivesAPanickingProvider(t *testing.T) {
	var failures []Failure
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Check let the provider's panic escape: %v", r)
			}
		}()
		failures = Check(build(&fake{caps: guardrails.Capabilities{ToolSegments: true}, panicOnTool: true}), opts)
	}()
	if len(failures) == 0 {
		t.Error("a provider that panics on tool segments passed")
	}
}

// A provider that ignores its context is reported within the suite's
// timeout, not waited on.
func TestAdversarial_ConformanceBoundsAHangingProvider(t *testing.T) {
	o := opts
	o.Timeout = 50 * time.Millisecond
	start := time.Now()
	if failures := Check(build(&fake{hang: 2 * time.Second}), o); len(failures) == 0 {
		t.Error("a provider that hangs past the timeout passed")
	}
	if took := time.Since(start); took > 6*time.Second {
		t.Errorf("Check took %v; it waits out a provider that ignores its context", took)
	}
}
