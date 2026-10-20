package declared

import "sync/atomic"

var (
	installed atomic.Pointer[Extractor]
	// builtin reads traceparent, the X-Tyk-* headers and the baggage
	// correlation keys; no compat family or harness extractor is enabled.
	builtin = New(Options{})
)

// SetDefault installs the Extractor the gateway runs on every captured
// request. Each runtime (the hub's embedded gateway, the microgateway)
// builds it from its configuration at start-up; nil restores the built-in
// one.
func SetDefault(x *Extractor) { installed.Store(x) }

// Default returns the installed Extractor, or the built-in one.
func Default() *Extractor {
	if x := installed.Load(); x != nil {
		return x
	}
	return builtin
}
