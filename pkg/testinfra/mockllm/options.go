package mockllm

import "time"

// Option is a functional option for configuring MockLLMBackend.
type Option func(*MockLLMBackend)

// WithLatency sets the simulated response latency.
func WithLatency(d time.Duration) Option {
	return func(m *MockLLMBackend) {
		m.latency = d
	}
}

// WithFailureRate sets the probability of returning a 500 error (0.0-1.0).
func WithFailureRate(rate float32) Option {
	return func(m *MockLLMBackend) {
		if rate < 0 {
			rate = 0
		}
		if rate > 1 {
			rate = 1
		}
		m.failureRate = rate
	}
}

// WithModel sets the model name for responses.
func WithModel(model string) Option {
	return func(m *MockLLMBackend) {
		m.model = model
	}
}

// WithVendor sets the vendor name.
func WithVendor(vendor string) Option {
	return func(m *MockLLMBackend) {
		m.vendor = vendor
	}
}
