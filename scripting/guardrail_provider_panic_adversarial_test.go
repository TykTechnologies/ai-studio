package scripting

// Adversarial test (tests agent, found testing 4uc.6): a guardrail provider
// that panics must not take the process down. The scan runs provider calls
// in goroutines of its own (guardrail_scan.go run), where neither net/http's
// per-request recovery nor the proxy's filter recovery reaches; a panic
// there ends the hub or the edge. The check runs in a child process so a
// crash fails this test instead of the whole package run.

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

type panickingProvider struct{}

func (panickingProvider) Capabilities() guardrails.Capabilities { return guardrails.Capabilities{} }

func (panickingProvider) Classify(context.Context, guardrails.Input) (guardrails.Verdict, error) {
	var m map[string]int
	m["boom"]++ // a provider bug: nil map write
	return guardrails.Verdict{}, nil
}

const panicChildEnv = "GUARDRAIL_PANIC_CHILD"

func TestAdversarial_ProviderPanicDoesNotCrashTheProcess(t *testing.T) {
	if os.Getenv(panicChildEnv) == "1" {
		f := providerFilter(t, panickingProvider{}, map[string]any{"fail_mode": guardrails.FailClosed})
		func() {
			// What the request goroutine's own recovery would catch.
			defer func() { _ = recover() }()
			out, err := NewFilterRunner(f).RunScript(&ScriptInput{
				RawInput: chatBody(t, chatMsg{Role: "user", Content: "hello"}), Format: "openai", VendorName: "openai", Context: app(1),
			}, nil)
			if err == nil && !out.Block {
				t.Error("a panicking provider let the request through under fail_mode closed")
			}
		}()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAdversarial_ProviderPanicDoesNotCrashTheProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), panicChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("a provider panic crashed the process (%v):\n%s", err, tail(string(out), 1500))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
