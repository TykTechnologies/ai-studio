package plugins

import "testing"

// A guardrail's judge call keeps its interaction type on the way to the hub;
// everything else an edge records is proxy traffic.
func TestPulseInteractionType(t *testing.T) {
	for in, want := range map[string]string{"guardrail_judge": "guardrail_judge", "": "proxy", "proxy": "proxy", "chat": "proxy"} {
		if got := pulseInteractionType(in); got != want {
			t.Errorf("pulseInteractionType(%q) = %q, want %q", in, got, want)
		}
	}
}
