package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertFilterSpec(name, event string) FilterSpec {
	return FilterSpec{Name: name, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":  guardrails.ProviderBuiltin,
		"detectors": []any{map[string]any{"name": guardrails.CategoryInjection}},
		"on_detect": guardrails.ActionAlert,
		"alert":     map[string]any{"event": event},
	}}
}

// Alert event names are unique across filters and exceptions.
func TestAlertEvents_UniqueAcrossFiltersAndExceptions(t *testing.T) {
	f := newExceptionFixture(t, "read_only")

	first, err := f.svc.CreateFilterFromSpec(alertFilterSpec("Alerting", "visor-pr-flag"))
	require.NoError(t, err)
	_, err = f.svc.CreateFilterFromSpec(alertFilterSpec("Another", "visor-pr-flag"))
	assert.ErrorIs(t, err, guardrails.ErrInvalidConfig, "a second filter with the name")
	assert.ErrorContains(t, err, `filter "Alerting"`)

	// Saving the same filter again keeps its own name.
	_, err = f.svc.UpdateFilterFromSpec(first.ID, alertFilterSpec("Alerting", "visor-pr-flag"))
	require.NoError(t, err)

	// An exception cannot take a filter's name, nor a filter an exception's.
	s := f.spec("text_user", guardrails.ActionAlert)
	s.AlertEvent = ptr("visor-pr-flag")
	_, err = f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
	assert.ErrorIs(t, err, ErrInvalidGuardrailException)

	s.AlertEvent = ptr("visor-softened")
	e, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
	require.NoError(t, err)
	assert.Equal(t, "visor-softened", e.AlertEvent)
	_, err = f.svc.CreateFilterFromSpec(alertFilterSpec("Third", "visor-softened"))
	assert.ErrorIs(t, err, guardrails.ErrInvalidConfig)
	assert.ErrorContains(t, err, "guardrail exception")

	// Nor another exception's.
	s2 := f.spec("text_system", guardrails.ActionAlert)
	s2.AlertEvent = ptr("visor-softened")
	_, err = f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s2)
	assert.ErrorIs(t, err, ErrInvalidGuardrailException)

	// Editing the exception itself keeps its name.
	_, err = f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asAdmin, GuardrailExceptionSpec{Justification: ptr("still PR vectors")})
	require.NoError(t, err)
}

func TestGuardrailException_AlertAction(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	s := f.spec("text_user", guardrails.ActionAlert)
	_, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
	assert.ErrorIs(t, err, ErrInvalidGuardrailException, "alert needs an event name")
	s.AlertEvent = ptr("Not A Slug")
	_, err = f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
	assert.ErrorIs(t, err, ErrInvalidGuardrailException)

	logSpec := f.spec("text_user", guardrails.ActionLog)
	logSpec.AlertEvent = ptr("visor")
	_, err = f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, logSpec)
	assert.ErrorIs(t, err, ErrInvalidGuardrailException, "alert_event only with alert")

	s.AlertEvent = ptr("visor")
	e, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, s)
	require.NoError(t, err)
	reviewed := e.ContentHash()

	// Renaming the alert changes what was reviewed (security .30).
	renamed, err := f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asWriter, GuardrailExceptionSpec{AlertEvent: ptr("visor-2")})
	require.NoError(t, err)
	assert.NotEqual(t, reviewed, renamed.ContentHash())
	_, err = f.svc.ApproveGuardrailException(f.app.ID, e.ID, f.admin.ID, reviewed)
	assert.ErrorIs(t, err, ErrGuardrailExceptionChanged)

	// Switching the action away from alert drops the name.
	logged, err := f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asWriter, GuardrailExceptionSpec{Action: ptr(guardrails.ActionLog)})
	require.NoError(t, err)
	assert.Equal(t, "", logged.AlertEvent)

	// An alert exception is within the floor wherever log is: not on an
	// acting app's text.
	_, err = f.svc.SetAppGuardrailCapability(f.app.ID, "acting", f.admin.ID)
	require.NoError(t, err)
	s3 := f.spec("text_system", guardrails.ActionAlert)
	s3.AlertEvent = ptr("visor-3")
	_, err = f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s3)
	assert.ErrorIs(t, err, ErrInvalidGuardrailException)
}
