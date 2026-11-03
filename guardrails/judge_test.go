package guardrails

import (
	"errors"
	"strings"
	"testing"
)

func judgeConfig(j Judge, shadow bool) Config {
	return Config{
		Provider:  ProviderBuiltin,
		Detectors: []DetectorConfig{{Name: CategoryInjection}},
		OnDetect:  ActionLog,
		Shadow:    shadow,
		Judge:     j,
	}
}

func TestNormalize_Judge(t *testing.T) {
	for _, c := range []struct {
		name    string
		judge   Judge
		shadow  bool
		wantErr string
		timeout int
	}{
		{name: "absent", judge: Judge{}},
		{name: "off keeps its LLM", judge: Judge{Mode: JudgeOff, LLMID: 4}},
		{name: "record defaults the timeout", judge: Judge{Mode: JudgeRecord, LLMID: 4}, timeout: DefaultJudgeTimeoutMs},
		{name: "soften with a timeout", judge: Judge{Mode: JudgeSoften, LLMID: 4, TimeoutMs: 1500}, timeout: 1500},
		{name: "shadow may record", judge: Judge{Mode: JudgeRecord, LLMID: 4}, shadow: true, timeout: DefaultJudgeTimeoutMs},
		{name: "shadow may not soften", judge: Judge{Mode: JudgeSoften, LLMID: 4}, shadow: true, wantErr: "may only record"},
		{name: "unknown mode", judge: Judge{Mode: "allow", LLMID: 4}, wantErr: "judge.mode"},
		{name: "on without an LLM", judge: Judge{Mode: JudgeRecord}, wantErr: "judge.llm_id"},
		{name: "negative timeout", judge: Judge{Mode: JudgeRecord, LLMID: 4, TimeoutMs: -1}, wantErr: "judge.timeout_ms"},
		{name: "timeout over the cap", judge: Judge{Mode: JudgeRecord, LLMID: 4, TimeoutMs: MaxJudgeTimeoutMs + 1}, wantErr: "judge.timeout_ms"},
		{name: "off with a negative timeout", judge: Judge{Mode: JudgeOff, TimeoutMs: -5}, wantErr: "judge.timeout_ms"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Normalize(judgeConfig(c.judge, c.shadow), false)
			if c.wantErr != "" {
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want ErrInvalidConfig mentioning %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Judge.TimeoutMs != c.timeout {
				t.Errorf("timeout_ms = %d, want %d", got.Judge.TimeoutMs, c.timeout)
			}
		})
	}
}

// A filter without a judge stores no judge key (edges and checksums see the
// same config as before), and one with a judge round-trips through storage.
func TestJudge_StoredOnlyWhenSet(t *testing.T) {
	cfg, err := Normalize(judgeConfig(Judge{}, false), false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := cfg.ToMap()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["judge"]; ok {
		t.Errorf("stored config has a judge: %v", m["judge"])
	}

	cfg, err = Normalize(judgeConfig(Judge{Mode: JudgeSoften, LLMID: 7, Model: "claude-haiku-5-5"}, false), false)
	if err != nil {
		t.Fatal(err)
	}
	m, err = cfg.ToMap()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseConfig(m)
	if err != nil {
		t.Fatal(err)
	}
	if back.Judge != (Judge{Mode: JudgeSoften, LLMID: 7, Model: "claude-haiku-5-5", TimeoutMs: DefaultJudgeTimeoutMs}) {
		t.Errorf("round trip = %+v", back.Judge)
	}
	if !back.Judge.Enabled() || back.Judge.Timeout() != DefaultJudgeTimeoutMs {
		t.Errorf("Enabled/Timeout = %v/%d", back.Judge.Enabled(), back.Judge.Timeout())
	}
	if (Judge{Mode: JudgeOff}).Enabled() || (Judge{}).Enabled() {
		t.Error("an off judge is enabled")
	}
}
