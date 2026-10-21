package probe

import (
	"fmt"
	"strings"
)

// AgentSpec renders an agent conversation instead of a single prompt: an
// OpenAI chat body whose tool results are noisy shell output (ANSI colour
// codes, trailing spaces, runs of blank lines, passing-test lines, a JSON
// listing), the shape request compressors work on. It is deterministic, so
// every request sends the same bytes.
type AgentSpec struct {
	// Turns is the number of tool calls and results.
	Turns int `yaml:"turns" json:"turns"`
	// ToolBytes is the total size of the tool results.
	ToolBytes int `yaml:"tool_bytes" json:"tool_bytes"`
}

// messages renders the conversation.
func (a AgentSpec) messages() []map[string]any {
	turns := max(a.Turns, 1)
	per := max(a.ToolBytes/turns, 256)
	msgs := []map[string]any{
		{"role": "system", "content": "You are a coding agent. Use the shell tool to run commands."},
		{"role": "user", "content": "Run the test suite and fix the failures."},
	}
	for t := 0; t < turns; t++ {
		id := fmt.Sprintf("call_%04d", t)
		msgs = append(msgs,
			map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": id, "type": "function",
				"function": map[string]any{"name": "shell", "arguments": fmt.Sprintf(`{"cmd":"pytest -q tests/suite_%d"}`, t)},
			}}},
			map[string]any{"role": "tool", "tool_call_id": id, "content": toolOutput(t, per)},
		)
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": "Continue."})
	return msgs
}

// toolOutput is about n bytes of noisy test-runner output for turn t.
func toolOutput(t, n int) string {
	var b strings.Builder
	b.Grow(n + 256)
	fmt.Fprintf(&b, "\x1b[1m============================= test session starts ==============================\x1b[0m   \n")
	fmt.Fprintf(&b, "platform linux -- Python 3.12.3, pytest-8.3.2\nrootdir: /srv/app\ncollected %d items\n\n\n\n", 400+t)
	i := 0
	for b.Len() < n*7/10 {
		fmt.Fprintf(&b, "tests/suite_%d/test_module_%03d.py::test_case_%04d \x1b[32mPASSED\x1b[0m        [%3d%%]   \n", t, i/20, i, i%100)
		if i%25 == 24 {
			b.WriteString("\n\n\n")
		}
		i++
	}
	b.WriteString("=================================== FAILURES ===================================\n")
	fmt.Fprintf(&b, "___ test_case_%04d ___\n\n    def test_case():\n>       assert ledger.balance(\"ACC-%d\") == 120\nE       AssertionError: assert 100 == 120\n\ntests/suite_%d/test_ledger.py:88: AssertionError\n", i, t, t)
	b.WriteString(`{"files": [`)
	for j := 0; b.Len() < n; j++ {
		if j > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"path": "src/module_%d/file_%d.py", "lines": %d, "covered": %d, "status": "ok"}`, t, j, 100+j%300, 90+j%200)
	}
	b.WriteString("]}\n")
	return b.String()
}
