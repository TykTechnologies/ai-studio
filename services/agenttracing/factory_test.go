package agenttracing

import (
	"context"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// fakeService embeds the inert service for the query methods.
type fakeService struct {
	communityService
	started bool
}

func (f *fakeService) Start(context.Context) error { f.started = true; return nil }
func (f *fakeService) Stop()                       {}
func (f *fakeService) Running() bool               { return true }
func (f *fakeService) IsEnterpriseAvailable() bool { return true }

func withFactory(t *testing.T, f FactoryFunc) {
	t.Helper()
	old := enterpriseFactory
	enterpriseFactory = f
	t.Cleanup(func() { enterpriseFactory = old; agenttrace.SetEnabled(false) })
}

func deps(enabled, licensed bool) Deps {
	cfg := config.DefaultAgentTracingConfig()
	cfg.Enabled = enabled
	return Deps{Config: cfg, LicenseValid: func() bool { return licensed }}
}

func assertInert(t *testing.T, svc Service, wantErr error, wantEnt bool) {
	t.Helper()
	if agenttrace.Enabled() {
		t.Fatal("process flag is on")
	}
	if _, r := agenttrace.Begin(context.Background()); r != nil {
		t.Fatal("a recorder was created while gated")
	}
	if svc.Running() || svc.IsEnterpriseAvailable() != wantEnt {
		t.Fatalf("running=%v enterprise=%v", svc.Running(), svc.IsEnterpriseAvailable())
	}
	cs, ok := svc.(communityService)
	if !ok || !errors.Is(cs.Err(), wantErr) {
		t.Fatalf("want inert service answering %v, got %#v", wantErr, svc)
	}
}

func TestCommunityStaysInertWhenEnabled(t *testing.T) {
	withFactory(t, nil)
	agenttrace.SetEnabled(true) // a stale flag must be cleared by the gate
	svc := NewService(deps(true, true))
	assertInert(t, svc, ErrEnterpriseFeature, false)
	if IsEnterpriseAvailable() {
		t.Fatal("CE reports enterprise available")
	}
}

func TestEnterpriseGate(t *testing.T) {
	called := 0
	withFactory(t, func(Deps) Service { called++; return &fakeService{} })

	t.Run("flag off", func(t *testing.T) {
		assertInert(t, NewService(deps(false, true)), ErrDisabled, true)
	})
	t.Run("no valid licence", func(t *testing.T) {
		assertInert(t, NewService(deps(true, false)), ErrDisabled, true)
	})
	t.Run("nil licence check", func(t *testing.T) {
		d := deps(true, true)
		d.LicenseValid = nil
		assertInert(t, NewService(d), ErrDisabled, true)
	})
	if called != 0 {
		t.Fatalf("factory ran %d times while gated", called)
	}
	t.Run("licensed and enabled", func(t *testing.T) {
		svc := NewService(deps(true, true))
		if !agenttrace.Enabled() || !svc.Running() || called != 1 {
			t.Fatalf("gate did not open: flag=%v running=%v called=%d", agenttrace.Enabled(), svc.Running(), called)
		}
	})
}

func TestEnterpriseFactoryReturningNil(t *testing.T) {
	withFactory(t, func(Deps) Service { return nil })
	assertInert(t, NewService(deps(true, true)), ErrDisabled, true)
}
