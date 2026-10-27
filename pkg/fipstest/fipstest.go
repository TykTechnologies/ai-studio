// Package fipstest is for tests that check the FIPS 140-3 build and mode.
//
// make test-fips (scripts/test-fips.sh) runs the test suites with
// GOFIPS140=v1.0.0 and GODEBUG=fips140=only, and sets CanaryEnv so that the
// canary tests (in this package and in the microgateway module) call
// RequireStrict. A canary that passes proves the run really linked the Go
// Cryptographic Module v1.0.0 (CMVP #5247) and enforced strict mode; one
// that fails or never ran means the run proves nothing.
package fipstest

import (
	"crypto/fips140"
	"os"
	"testing"
)

// CanaryEnv is set to 1 by make test-fips. The canary tests skip without it.
const CanaryEnv = "FIPS_STRICT_CANARY"

// ModuleVersion is the Go Cryptographic Module version FIPS builds pin
// (GOFIPS140=v1.0.0).
const ModuleVersion = "v1.0.0"

// RequireStrict fails t unless this test binary runs the Go Cryptographic
// Module ModuleVersion with fips140=only enforced.
func RequireStrict(t testing.TB) {
	t.Helper()
	if !fips140.Enabled() {
		t.Fatal("FIPS 140-3 mode is not enabled (want GOFIPS140=v1.0.0 and GODEBUG=fips140=only)")
	}
	if !fips140.Enforced() {
		t.Fatal("FIPS 140-3 mode is enabled but not enforced (want GODEBUG=fips140=only)")
	}
	if v := fips140.Version(); v != ModuleVersion {
		t.Fatalf("Go Cryptographic Module version is %q, want %q (GOFIPS140=%s)", v, ModuleVersion, ModuleVersion)
	}
}

// Canary is the body of a canary test: RequireStrict under make test-fips,
// a skip otherwise.
func Canary(t *testing.T) {
	t.Helper()
	if os.Getenv(CanaryEnv) != "1" {
		t.Skipf("%s is not set; this canary runs under make test-fips", CanaryEnv)
	}
	RequireStrict(t)
}
