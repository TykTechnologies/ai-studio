package guardrails

import "fmt"

// Judge configures a guardrail's second opinion: a gateway-managed LLM asked
// whether a segment the filter would block or quarantine is in fact benign
// for the app that sent it (features/GuardrailJudge.md). It reads the
// flagged, attacker-controlled text, so it may only ever soften a verdict,
// never below the app's capability floor, and any failure keeps the original
// verdict.
type Judge struct {
	// Mode is JudgeOff (or empty), JudgeRecord (ask, record the opinion
	// beside the verdict, change nothing) or JudgeSoften (ask inline and let
	// a benign opinion soften the verdict).
	Mode string `json:"mode,omitempty"`
	// LLMID is the judge: an active, chat-capable LLM configured in AI
	// Studio. The call goes straight to its vendor, never through the proxy.
	LLMID uint `json:"llm_id,omitempty"`
	// Model is the judge's model; empty means the LLM's default model.
	Model string `json:"model,omitempty"`
	// TimeoutMs bounds one judge call; 0 means DefaultJudgeTimeoutMs. A call
	// that runs out keeps the original verdict.
	TimeoutMs int `json:"timeout_ms,omitempty"`
}

// Judge modes (Judge.Mode).
const (
	JudgeOff    = "off"
	JudgeRecord = "record"
	JudgeSoften = "soften"
)

// Judge timeout default and bounds.
const (
	DefaultJudgeTimeoutMs = 3000
	MaxJudgeTimeoutMs     = 30_000
)

// Enabled reports whether the judge is asked at all.
func (j Judge) Enabled() bool {
	return j.Mode == JudgeRecord || j.Mode == JudgeSoften
}

// Timeout is the judge's per-call timeout in milliseconds.
func (j Judge) Timeout() int {
	if j.TimeoutMs > 0 {
		return j.TimeoutMs
	}
	return DefaultJudgeTimeoutMs
}

// normalize checks the judge settings. A judge that is off is left as it is
// (its LLM may be kept for later), so a filter without one stores nothing new.
// A shadow filter never changes traffic, so its judge may only record.
func (j *Judge) normalize(shadow bool) error {
	switch j.Mode {
	case "", JudgeOff:
		if j.TimeoutMs < 0 {
			return fmt.Errorf("%w: judge.timeout_ms must not be negative", ErrInvalidConfig)
		}
		return nil
	case JudgeRecord:
	case JudgeSoften:
		if shadow {
			return fmt.Errorf("%w: a shadow filter never changes traffic, so its judge may only record (judge.mode record)", ErrInvalidConfig)
		}
	default:
		return fmt.Errorf("%w: judge.mode %q is not one of off, record, soften", ErrInvalidConfig, j.Mode)
	}
	if j.LLMID == 0 {
		return fmt.Errorf("%w: judge.llm_id is required when the judge is on", ErrInvalidConfig)
	}
	if j.TimeoutMs < 0 || j.TimeoutMs > MaxJudgeTimeoutMs {
		return fmt.Errorf("%w: judge.timeout_ms must be between 0 and %d", ErrInvalidConfig, MaxJudgeTimeoutMs)
	}
	if j.TimeoutMs == 0 {
		j.TimeoutMs = DefaultJudgeTimeoutMs
	}
	return nil
}
