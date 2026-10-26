package fipstest

import (
	"crypto/fips140"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Cross-build tests check that data written by one build is read by
// another: a standard build (no GOFIPS140) and a FIPS build
// (GOFIPS140=ModuleVersion) of the same test package, run one after the
// other against the same database. The parent test compiles both binaries
// with BuildTestBinaries and runs one phase test at a time in either with
// RunPhase; a phase test calls Phase to learn whether it was asked to run.

// PhaseEnv names the phase test a RunPhase child runs; phase tests skip
// without it.
const PhaseEnv = "FIPS_CROSSBUILD_PHASE"

// modeEnv carries the Mode to the child, which Phase checks against the
// process's actual FIPS 140-3 state.
const modeEnv = "FIPS_CROSSBUILD_MODE"

// Mode is how a phase's binary runs: the standard build, or the FIPS build
// with GODEBUG=fips140=on (production) or fips140=only (strict).
type Mode string

const (
	Standard   Mode = "std"
	FIPSOn     Mode = "on"
	FIPSStrict Mode = "only"
)

// Binaries are a test package compiled as a standard and as a FIPS build.
type Binaries struct {
	Std, FIPS string
}

// BuildTestBinaries compiles the test binary of the package in dir (a
// directory, as for go test -c) as a standard build and as a FIPS build.
// It needs the go command; build output goes to t's log on failure.
func BuildTestBinaries(t testing.TB, dir string) Binaries {
	t.Helper()
	out := t.TempDir()
	build := func(name, gofips string) string {
		bin := filepath.Join(out, name)
		cmd := exec.Command("go", "test", "-c", "-o", bin, ".")
		cmd.Dir = dir
		cmd.Env = withEnv(os.Environ(), "GOFIPS140="+gofips)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go test -c (GOFIPS140=%s) in %s: %v\n%s", gofips, dir, err, b)
		}
		return bin
	}
	return Binaries{Std: build("std.test", "off"), FIPS: build("fips.test", ModuleVersion)}
}

// RunPhase runs the phase test named test (an exact test name) in the
// binary for mode, with env added, and fails t if it fails. The child's
// output is logged.
func RunPhase(t testing.TB, bins Binaries, mode Mode, test string, env ...string) {
	t.Helper()
	bin, godebug := bins.FIPS, "fips140="+string(mode)
	if mode == Standard {
		bin, godebug = bins.Std, "fips140=off"
	}
	cmd := exec.Command(bin, "-test.run", "^"+test+"$", "-test.count=1", "-test.v")
	cmd.Env = withEnv(os.Environ(), append([]string{PhaseEnv + "=" + test, modeEnv + "=" + string(mode), "GODEBUG=" + godebug}, env...)...)
	b, err := cmd.CombinedOutput()
	t.Logf("%s (%s):\n%s", test, mode, b)
	if err != nil {
		t.Fatalf("phase %s (%s) failed: %v", test, mode, err)
	}
	if !strings.Contains(string(b), "--- PASS: "+test) {
		t.Fatalf("phase %s (%s) did not run", test, mode)
	}
}

// Phase returns the mode a RunPhase child runs in, and skips t when t is
// not the phase RunPhase asked for (a normal go test run). It fails t when
// the process's FIPS 140-3 state does not match that mode.
func Phase(t *testing.T) Mode {
	t.Helper()
	if os.Getenv(PhaseEnv) != t.Name() {
		t.Skipf("phase test; run by its cross-build parent test (%s)", PhaseEnv)
	}
	want := Mode(os.Getenv(modeEnv))
	switch want {
	case Standard:
		if fips140.Enabled() {
			t.Fatal("phase wants the standard build, but FIPS 140-3 mode is enabled")
		}
	case FIPSOn, FIPSStrict:
		if !fips140.Enabled() || fips140.Version() != ModuleVersion {
			t.Fatalf("phase wants a FIPS build: enabled=%v version=%q", fips140.Enabled(), fips140.Version())
		}
		if fips140.Enforced() != (want == FIPSStrict) {
			t.Fatalf("phase wants fips140=%s, but Enforced()=%v", want, fips140.Enforced())
		}
	default:
		t.Fatalf("unknown mode %q", want)
	}
	return want
}

// withEnv returns env with each KEY=value of set replacing any earlier
// value of KEY.
func withEnv(env []string, set ...string) []string {
	out := make([]string, 0, len(env)+len(set))
	for _, kv := range env {
		key := kv[:strings.IndexByte(kv+"=", '=')]
		replaced := false
		for _, s := range set {
			if strings.HasPrefix(s, key+"=") {
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, kv)
		}
	}
	return append(out, set...)
}
