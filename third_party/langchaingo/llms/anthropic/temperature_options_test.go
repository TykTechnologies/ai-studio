package anthropic

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

func callOptions(options ...llms.CallOption) *llms.CallOptions {
	opts := &llms.CallOptions{}
	for _, option := range options {
		option(opts)
	}
	return opts
}

func TestTemperatureFromOptions(t *testing.T) {
	if got := temperatureFromOptions(callOptions()); got != nil {
		t.Errorf("no WithTemperature: got %v, want nil", *got)
	}

	// 0 is a valid temperature, and must survive the round trip rather than
	// being mistaken for the unset zero value.
	got := temperatureFromOptions(callOptions(llms.WithTemperature(0)))
	if got == nil {
		t.Fatal("WithTemperature(0) was dropped")
	}
	if *got != 0 {
		t.Errorf("got %v, want 0", *got)
	}

	got = temperatureFromOptions(callOptions(llms.WithTemperature(0.7)))
	if got == nil || *got != 0.7 {
		t.Errorf("got %v, want 0.7", got)
	}

	if got := temperatureFromOptions(nil); got != nil {
		t.Errorf("nil options: got %v, want nil", *got)
	}
}
