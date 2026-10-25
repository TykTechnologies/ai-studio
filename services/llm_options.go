package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// LLMOption sets an LLM attribute that CreateLLM, CreateLLMWithNamespace and
// UpdateLLM take outside their positional parameters. It runs before
// validation and the before-* object hooks; an error aborts the call. On
// UpdateLLM an attribute without an option keeps its stored value.
type LLMOption func(*models.LLM) error

// WithVendorWorkspaceID pins the vendor workspace the gateway calls the LLM
// in ("" removes the pin).
func WithVendorWorkspaceID(id string) LLMOption {
	return func(l *models.LLM) error {
		if err := models.ValidateVendorWorkspaceID(id); err != nil {
			return &LLMValidationError{Err: err}
		}
		l.VendorWorkspaceID = id
		return nil
	}
}

// WithPriceMultipliers sets the LLM's per-tier price multiplier overrides
// (nil or empty removes them).
func WithPriceMultipliers(m models.LLMPriceMultipliers) LLMOption {
	return func(l *models.LLM) error {
		if err := m.Validate(); err != nil {
			return &LLMValidationError{Err: err}
		}
		if len(m) == 0 {
			m = nil
		}
		l.PriceMultipliers = m
		return nil
	}
}

// WithLLMTracing sets the LLM's agent tracing content mode cap ("" for no
// cap) and whether traceparent propagation to its upstream is switched off.
func WithLLMTracing(contentModeMax string, propagationOptOut bool) LLMOption {
	return func(l *models.LLM) error {
		if err := models.ValidateTracingContentMode("tracing_content_mode_max", contentModeMax); err != nil {
			return &LLMValidationError{Err: err}
		}
		l.TracingContentModeMax = contentModeMax
		l.TracingPropagationOptOut = propagationOptOut
		return nil
	}
}

// WithAllowInternalHost sets the LLM's "Allow internal host" switch: its
// own endpoint may be an internal address (netguard.ExemptionFor). The API
// lets only an administrator change it.
func WithAllowInternalHost(allow bool) LLMOption {
	return func(l *models.LLM) error {
		l.AllowInternalHost = allow
		return nil
	}
}

// LLMValidationError is an LLM attribute the caller got wrong (a 400).
type LLMValidationError struct{ Err error }

func (e *LLMValidationError) Error() string { return e.Err.Error() }
func (e *LLMValidationError) Unwrap() error { return e.Err }

func applyLLMOptions(l *models.LLM, opts []LLMOption) error {
	for _, opt := range opts {
		if err := opt(l); err != nil {
			return err
		}
	}
	return nil
}
