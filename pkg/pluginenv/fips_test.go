package pluginenv

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildPlugin compiles a trivial Go program, with GOFIPS140=v1.0.0 when fips.
func buildPlugin(t *testing.T, fips bool) string {
	t.Helper()
	if fips {
		return buildPluginWith(t, "v1.0.0")
	}
	return buildPluginWith(t, "off")
}

func buildPluginWith(t *testing.T, gofips140 string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds Go binaries")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module plugintest\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))
	out := filepath.Join(dir, "plugin")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	cmd.Env = append(cmd.Env, "GOFIPS140="+gofips140)
	b, err := cmd.CombinedOutput()
	require.NoError(t, err, string(b))
	return out
}

func TestFIPSBuild(t *testing.T) {
	ok, why := FIPSBuild(buildPlugin(t, true))
	assert.True(t, ok, why)

	ok, why = FIPSBuild(buildPlugin(t, false))
	assert.False(t, ok)
	assert.Equal(t, "built without GOFIPS140", why)

	// "latest" is the toolchain's module, not the CMVP-validated v1.0.0.
	ok, why = FIPSBuild(buildPluginWith(t, "latest"))
	assert.False(t, ok)
	assert.Equal(t, "built with GOFIPS140=latest, not the validated v1.0.0", why)

	script := filepath.Join(t.TempDir(), "plugin.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755))
	ok, why = FIPSBuild(script)
	assert.False(t, ok)
	assert.Contains(t, why, "not a readable Go binary")
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })
	return &buf
}

func forceFIPS(t *testing.T, on bool) {
	t.Helper()
	prev := fipsEnabled
	fipsEnabled = func() bool { return on }
	t.Cleanup(func() { fipsEnabled = prev })
}

// A FIPS-mode host warns when it starts a non-FIPS plugin, and only then.
func TestCommandWarnsOnNonFIPSPluginInFIPSMode(t *testing.T) {
	std, fips := buildPlugin(t, false), buildPlugin(t, true)

	buf := captureLog(t)
	forceFIPS(t, true)
	Command(std)
	assert.Contains(t, buf.String(), "not a FIPS build")
	assert.Contains(t, buf.String(), "built without GOFIPS140")

	buf.Reset()
	Command(fips)
	assert.NotContains(t, buf.String(), "not a FIPS build")

	forceFIPS(t, false)
	buf.Reset()
	Command(std)
	assert.NotContains(t, buf.String(), "not a FIPS build", "standard hosts do not check")
}
