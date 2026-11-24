// Package makefile holds checks of Makefile logic that no build exercises.
package makefile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// licenseLine returns the recipe line of test-package-smoke-ent that sets
// TYK_AI_LICENSE, as it stands in the repository's Makefile.
func licenseLine(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	inTarget := false
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "test-package-smoke-ent:"):
			inTarget = true
		case inTarget && line != "" && !strings.HasPrefix(line, "\t"):
			inTarget = false
		case inTarget && strings.Contains(line, "export TYK_AI_LICENSE="):
			return line
		}
	}
	t.Fatal("test-package-smoke-ent no longer has a line that exports TYK_AI_LICENSE; update this test")
	return ""
}

// The Enterprise package smoke (make test-package-smoke-ent, the release
// gate's "Package Smoke Tests (ENT)") must run with the license the
// environment gives it. Its recipe read the license from dev/.env.secrets
// and exported the result unconditionally: in CI, where that file does not
// exist, it exported an empty value over the secret the workflow had set,
// and both services in the smoke container exited at start. The file is a
// fallback for local runs only.
//
// The line is taken from the Makefile and run in a scratch directory with
// placeholder values; no real license is involved.
func TestPackageSmokeKeepsTheLicenseFromTheEnvironment(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is not installed")
	}
	line := licenseLine(t)

	cases := []struct {
		name string
		env  string // TYK_AI_LICENSE in the environment; "" for unset
		file string // TYK_AI_LICENSE in dev/.env.secrets; "" for no file
		want string
	}{
		{"CI: set in the environment, no secrets file", "from-environment", "", "from-environment"},
		{"set in both: the environment wins", "from-environment", "from-file", "from-environment"},
		{"local: only in the secrets file", "", "from-file", "from-file"},
		{"nowhere", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			probe := "probe:\n" + line + "\n\t@printf '%s' \"$$TYK_AI_LICENSE\"\n"
			if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(probe), 0o644); err != nil {
				t.Fatal(err)
			}
			if tc.file != "" {
				if err := os.MkdirAll(filepath.Join(dir, "dev"), 0o755); err != nil {
					t.Fatal(err)
				}
				secrets := "OTHER=x\nTYK_AI_LICENSE=" + tc.file + "\n"
				if err := os.WriteFile(filepath.Join(dir, "dev", ".env.secrets"), []byte(secrets), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.Command("make", "--no-print-directory", "probe")
			cmd.Dir = dir
			for _, kv := range os.Environ() {
				// Not the caller's license, and not its make flags.
				if !strings.HasPrefix(kv, "TYK_AI_LICENSE=") && !strings.HasPrefix(kv, "MAKEFLAGS=") && !strings.HasPrefix(kv, "MAKELEVEL=") {
					cmd.Env = append(cmd.Env, kv)
				}
			}
			if tc.env != "" {
				cmd.Env = append(cmd.Env, "TYK_AI_LICENSE="+tc.env)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make failed: %v\n%s", err, out)
			}
			if got := string(out); got != tc.want {
				t.Errorf("the recipe sees TYK_AI_LICENSE = %q, want %q", got, tc.want)
			}
		})
	}
}
