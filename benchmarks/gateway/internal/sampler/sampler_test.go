package sampler

import (
	"strings"
	"testing"
)

func TestParseProm(t *testing.T) {
	text := `# HELP go_goroutines Number of goroutines.
# TYPE go_goroutines gauge
go_goroutines 42
process_resident_memory_bytes 1.2345e+08
go_gc_duration_seconds{quantile="0.5"} 0.0001
go_gc_duration_seconds_count 17
aistudio_llm_requests_total{llm="x"} 5
`
	m := ParseProm(strings.NewReader(text), gatewayMetrics)
	if m["go_goroutines"] != 42 || m["process_resident_memory_bytes"] != 1.2345e8 || m["go_gc_duration_seconds_count"] != 17 {
		t.Fatalf("%v", m)
	}
	if _, ok := m["aistudio_llm_requests_total"]; ok {
		t.Fatal("unrequested metric included")
	}
}
