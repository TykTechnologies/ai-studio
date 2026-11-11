package metrics

import (
	"strings"
	"sync/atomic"
	"testing"
)

// A labelled gauge function is read at scrape time and exports one series
// per key of the map it returns.
func TestRegisterLabelledGaugeFunc(t *testing.T) {
	h := Init()
	var current atomic.Value
	current.Store("cp-a:50051")
	err := RegisterLabelledGaugeFunc("test_labelled_gauge_connected", "test gauge", "endpoint", func() map[string]float64 {
		values := map[string]float64{"cp-a:50051": 0, "cp-b:50051": 0}
		values[current.Load().(string)] = 1
		return values
	})
	if err != nil {
		t.Fatalf("RegisterLabelledGaugeFunc: %v", err)
	}

	body := scrape(t, h)
	for _, want := range []string{
		`test_labelled_gauge_connected{endpoint="cp-a:50051"} 1`,
		`test_labelled_gauge_connected{endpoint="cp-b:50051"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}

	current.Store("cp-b:50051")
	body = scrape(t, h)
	for _, want := range []string{
		`test_labelled_gauge_connected{endpoint="cp-a:50051"} 0`,
		`test_labelled_gauge_connected{endpoint="cp-b:50051"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("after the move, scrape lacks %q", want)
		}
	}
}
