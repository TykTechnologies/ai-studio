package guardrails

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func budgetConfig(b Budget, timeoutMs int) Config {
	return Config{
		Provider:  ProviderBuiltin,
		Detectors: []DetectorConfig{{Name: CategoryInjection}},
		OnDetect:  ActionBlock,
		TimeoutMs: timeoutMs,
		Budget:    b,
	}
}

func TestNormalizeBudget(t *testing.T) {
	cfg, err := Normalize(budgetConfig(Budget{}, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	want := Budget{MaxScanChars: DefaultBudgetMaxScanChars, MaxPieces: DefaultBudgetMaxPieces, TimeMs: BudgetTimeoutMultiple * DefaultTimeoutMs, SingleCallChars: DefaultBudgetSingleCallChars}
	if cfg.Budget != want {
		t.Errorf("defaults = %+v, want %+v", cfg.Budget, want)
	}
	if !cfg.CacheEnabled() {
		t.Error("the cache is on unless switched off")
	}

	// A long per-call timeout raises the budget's floor rather than failing.
	if cfg, err := Normalize(budgetConfig(Budget{}, MaxBudgetTimeMs+1000), false); err != nil || cfg.Budget.TimeMs != MaxBudgetTimeMs+1000 {
		t.Errorf("long timeout: budget %+v, err %v", cfg.Budget, err)
	}

	for name, b := range map[string]Budget{
		"negative scan chars":    {MaxScanChars: -1},
		"too many scan chars":    {MaxScanChars: MaxBudgetScanChars + 1},
		"negative pieces":        {MaxPieces: -1},
		"too many pieces":        {MaxPieces: MaxBudgetPieces + 1},
		"time under the timeout": {TimeMs: 100},
		"too much time":          {TimeMs: MaxBudgetTimeMs + 1},
		"negative single call":   {SingleCallChars: -1},
		"single call over scan":  {MaxScanChars: 1000, SingleCallChars: 1001},
	} {
		if _, err := Normalize(budgetConfig(b, 2000), false); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: err %v, want ErrInvalidConfig", name, err)
		}
	}

	// The single-call bound follows a lower scan cap.
	if cfg, err := Normalize(budgetConfig(Budget{MaxScanChars: 1000}, 0), false); err != nil || cfg.Budget.SingleCallChars != 1000 {
		t.Errorf("low scan cap: budget %+v, err %v", cfg.Budget, err)
	}

	off := false
	c := budgetConfig(Budget{}, 0)
	c.Cache = &off
	if c.CacheEnabled() {
		t.Error("cache: false switches the cache off")
	}
}

// The budget and the cache switch survive the trip to the edge.
func TestBudgetConfigRoundTripsToTheEdge(t *testing.T) {
	raw := map[string]any{
		"provider": ProviderBuiltin, "detectors": []any{map[string]any{"name": CategoryInjection}}, "on_detect": ActionBlock,
		"budget": map[string]any{"max_scan_chars": 1000, "max_pieces": 7, "time_ms": 9000, "single_call_chars": 500}, "cache": false,
	}
	js, err := ConfigJSONForEdge(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ConfigFromJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(m)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = Normalize(cfg, false); err != nil {
		t.Fatal(err)
	}
	if cfg.Budget != (Budget{MaxScanChars: 1000, MaxPieces: 7, TimeMs: 9000, SingleCallChars: 500}) || cfg.CacheEnabled() {
		t.Errorf("edge config: budget %+v, cache %v", cfg.Budget, cfg.CacheEnabled())
	}
}

func TestErrorClass(t *testing.T) {
	for err, want := range map[error]string{
		&StatusError{Status: 400}:                      "input_rejected",
		&StatusError{Status: 413}:                      "input_rejected",
		fmt.Errorf("x: %w", &StatusError{Status: 422}): "input_rejected",
		&StatusError{Status: 429}:                      "http_429",
		&StatusError{Status: 503}:                      "http_503",
		&StatusError{Status: 401}:                      "http_401",
		fmt.Errorf("x: %w", ErrInputRejected):          "input_rejected",
		fmt.Errorf("x: %w", context.DeadlineExceeded):  "timeout",
		context.Canceled:                               "canceled",
		errors.New("dial tcp: connection refused"):     "error",
	} {
		if got := ErrorClass(err); got != want {
			t.Errorf("ErrorClass(%v) = %q, want %q", err, got, want)
		}
	}
	if (&StatusError{Status: 400}).Error() != "provider returned HTTP 400" {
		t.Error("StatusError text carries more than the status")
	}
}
