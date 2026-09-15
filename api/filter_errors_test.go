package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// The status a filter error maps to comes from the sentinel it wraps, never
// from its text, so a reworded message cannot turn a 400 into a 500.
func TestFilterErrorStatus_UsesSentinels(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"not found":                 {gorm.ErrRecordNotFound, http.StatusNotFound},
		"wrapped not found":         {fmt.Errorf("loading: %w", gorm.ErrRecordNotFound), http.StatusNotFound},
		"invalid config":            {fmt.Errorf("%w: reworded entirely", guardrails.ErrInvalidConfig), http.StatusBadRequest},
		"invalid config, twice":     {fmt.Errorf("guardrail 'x': %w", fmt.Errorf("%w: nope", guardrails.ErrInvalidConfig)), http.StatusBadRequest},
		"unknown provider":          {fmt.Errorf("%w: %q", guardrails.ErrUnknownProvider, "nope"), http.StatusBadRequest},
		"invalid spec":              {fmt.Errorf("%w: kind is odd", services.ErrInvalidFilterSpec), http.StatusBadRequest},
		"anything else is ours":     {errors.New("guardrail config: looks like a user error but is not wrapped"), http.StatusInternalServerError},
		"database failure":          {errors.New("connection refused"), http.StatusInternalServerError},
		"message mentions required": {errors.New("column is required"), http.StatusInternalServerError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, filterErrorStatus(tc.err))
		})
	}
}

// The real validation paths wrap the sentinels, so the mapping above holds
// for what the service actually returns.
func TestFilterValidationErrorsWrapSentinels(t *testing.T) {
	_, err := guardrails.ParseConfig(nil)
	assert.ErrorIs(t, err, guardrails.ErrInvalidConfig)

	_, err = guardrails.Normalize(guardrails.Config{Provider: "builtin"}, false)
	assert.ErrorIs(t, err, guardrails.ErrInvalidConfig, "no detectors")

	_, err = guardrails.Normalize(guardrails.Config{Provider: "nope", Detectors: []guardrails.DetectorConfig{{Name: "x"}}}, false)
	assert.ErrorIs(t, err, guardrails.ErrUnknownProvider)
}
