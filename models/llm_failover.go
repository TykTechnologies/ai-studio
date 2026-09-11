package models

import (
	"database/sql/driver"
)

// Failover trigger defaults. A fallback is attempted when the primary's
// upstream answers with one of these statuses, times out, or cannot be
// reached at all. 4xx codes other than 408/429 are caller or configuration
// problems that every target would repeat, so they are never failover
// triggers; the validator rejects them.
var DefaultFailoverStatusCodes = []int{408, 429, 500, 502, 503, 504}

// MaxFailoverTargets bounds the waterfall so a misconfiguration cannot turn
// one request into an unbounded chain of upstream calls.
const MaxFailoverTargets = 10

// LLMFailoverTarget is one rung of the waterfall: which LLM entry to try and
// which model to ask it for. Model is required and must satisfy the target
// LLM's allowed-models list; that is checked when the primary is saved and
// again at request time, because the target's list can change later.
type LLMFailoverTarget struct {
	LLMID uint   `json:"llm_id"`
	Model string `json:"model"`
}

// LLMFailoverTriggers narrows when the waterfall is consulted. Nil pointers
// mean "use the default", so a stored config that predates a new knob keeps
// the documented behaviour.
type LLMFailoverTriggers struct {
	StatusCodes          []int `json:"status_codes,omitempty"`
	OnTimeout            *bool `json:"on_timeout,omitempty"`
	OnConnectionError    *bool `json:"on_connection_error,omitempty"`
	AttemptTimeoutSecond int   `json:"attempt_timeout_seconds,omitempty"`
}

// LLMFailover is the ordered waterfall tried when an LLM's upstream fails.
// It is stored as a JSON column on the LLM row. An empty Targets list means
// no failover and is persisted as SQL NULL, so untouched LLMs stay NULL.
//
// Only the primary's waterfall is consulted for a request; a fallback's own
// waterfall is never followed, so chains cannot loop.
type LLMFailover struct {
	Targets  []LLMFailoverTarget  `json:"targets"`
	Triggers *LLMFailoverTriggers `json:"triggers,omitempty"`
}

// Enabled reports whether there is anything to fall over to.
func (f LLMFailover) Enabled() bool {
	return len(f.Targets) > 0
}

// ResolvedFailoverTriggers is LLMFailoverTriggers with every default applied,
// in the shape the proxy wants to consult per attempt.
type ResolvedFailoverTriggers struct {
	StatusCodes          map[int]bool
	OnTimeout            bool
	OnConnectionError    bool
	AttemptTimeoutSecond int // 0 = use the proxy's LLM timeout
}

// EffectiveTriggers applies the defaults. Status codes below 500 other than
// 408 and 429 are dropped here as well as at validation time, so a row that
// bypassed the API cannot make a 403 fail over.
func (f LLMFailover) EffectiveTriggers() ResolvedFailoverTriggers {
	out := ResolvedFailoverTriggers{
		StatusCodes:       map[int]bool{},
		OnTimeout:         true,
		OnConnectionError: true,
	}
	codes := DefaultFailoverStatusCodes
	if f.Triggers != nil {
		if len(f.Triggers.StatusCodes) > 0 {
			codes = f.Triggers.StatusCodes
		}
		if f.Triggers.OnTimeout != nil {
			out.OnTimeout = *f.Triggers.OnTimeout
		}
		if f.Triggers.OnConnectionError != nil {
			out.OnConnectionError = *f.Triggers.OnConnectionError
		}
		if f.Triggers.AttemptTimeoutSecond > 0 {
			out.AttemptTimeoutSecond = f.Triggers.AttemptTimeoutSecond
		}
	}
	for _, c := range codes {
		if FailoverStatusCodeAllowed(c) {
			out.StatusCodes[c] = true
		}
	}
	return out
}

// FailoverStatusCodeAllowed reports whether a status may act as a failover
// trigger: any 5xx, plus 408 (request timeout) and 429 (rate limited).
func FailoverStatusCodeAllowed(code int) bool {
	if code >= 500 && code <= 599 {
		return true
	}
	return code == 408 || code == 429
}

// Scan implements sql.Scanner. A NULL column is an empty waterfall.
func (f *LLMFailover) Scan(value interface{}) error {
	if value == nil {
		*f = LLMFailover{}
		return nil
	}
	return JSONScan(value, f)
}

// Value implements driver.Valuer. No targets is stored as NULL rather than
// as "{}" so existing rows and LLMs that never had a waterfall look the same.
func (f LLMFailover) Value() (driver.Value, error) {
	if !f.Enabled() {
		return nil, nil
	}
	return JSONValue(f)
}
