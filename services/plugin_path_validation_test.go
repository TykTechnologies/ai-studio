package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #397: plugin executables must be validated before being handed to
// exec.Command — scope checks against an allowlisted plugin directory, no
// traversal, and basic sanity (exists, regular file, executable).

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	return p
}

func TestValidatePluginExecutablePathBasics(t *testing.T) {
	t.Setenv(pluginAllowedDirsEnv, "")

	dir := t.TempDir()
	exe := writeExecutable(t, dir, "plugin-bin")

	t.Run("valid executable passes and returns the resolved path", func(t *testing.T) {
		got, err := validatePluginExecutablePath(exe)
		require.NoError(t, err)
		want, err := filepath.EvalSymlinks(exe)
		require.NoError(t, err)
		assert.Equal(t, want, got, "the returned path must be the fully resolved path that was validated")
		assert.True(t, filepath.IsAbs(got))
	})

	t.Run("bare command name resolves via PATH like exec.Command", func(t *testing.T) {
		t.Setenv("PATH", dir)
		got, err := validatePluginExecutablePath("plugin-bin")
		require.NoError(t, err)
		want, err := filepath.EvalSymlinks(exe)
		require.NoError(t, err)
		assert.Equal(t, want, got, "bare names must be resolved through PATH, matching exec.Command semantics")
	})

	t.Run("bare command name not on PATH rejected", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := validatePluginExecutablePath("plugin-bin")
		assert.Error(t, err)
	})

	t.Run("empty path rejected", func(t *testing.T) {
		_, err := validatePluginExecutablePath("")
		assert.Error(t, err)
	})

	t.Run("nonexistent path rejected", func(t *testing.T) {
		_, err := validatePluginExecutablePath(filepath.Join(dir, "missing"))
		assert.Error(t, err)
	})

	t.Run("directory rejected", func(t *testing.T) {
		_, err := validatePluginExecutablePath(dir)
		assert.Error(t, err)
	})

	t.Run("non-executable file rejected", func(t *testing.T) {
		p := filepath.Join(dir, "not-exec")
		require.NoError(t, os.WriteFile(p, []byte("data"), 0o644))
		_, err := validatePluginExecutablePath(p)
		assert.Error(t, err)
	})
}

func TestValidatePluginExecutablePathAllowedDirs(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	inside := writeExecutable(t, allowed, "good-plugin")
	escaped := writeExecutable(t, outside, "evil-plugin")

	t.Setenv(pluginAllowedDirsEnv, allowed)

	t.Run("inside allowed dir passes", func(t *testing.T) {
		got, err := validatePluginExecutablePath(inside)
		require.NoError(t, err)
		want, err := filepath.EvalSymlinks(inside)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("outside allowed dir rejected", func(t *testing.T) {
		_, err := validatePluginExecutablePath(escaped)
		assert.Error(t, err)
	})

	t.Run("traversal out of allowed dir rejected", func(t *testing.T) {
		sneaky := filepath.Join(allowed, "..", filepath.Base(outside), "evil-plugin")
		_, err := validatePluginExecutablePath(sneaky)
		assert.Error(t, err)
	})

	t.Run("symlink escaping allowed dir rejected", func(t *testing.T) {
		link := filepath.Join(allowed, "link-to-evil")
		require.NoError(t, os.Symlink(escaped, link))
		_, err := validatePluginExecutablePath(link)
		assert.Error(t, err)
	})

	t.Run("multiple allowed dirs honored", func(t *testing.T) {
		t.Setenv(pluginAllowedDirsEnv, outside+string(os.PathListSeparator)+allowed)
		got, err := validatePluginExecutablePath(escaped)
		require.NoError(t, err)
		want, err := filepath.EvalSymlinks(escaped)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestValidatePluginExecutablePathTraversalWithoutAllowlist(t *testing.T) {
	t.Setenv(pluginAllowedDirsEnv, "")

	dir := t.TempDir()
	writeExecutable(t, dir, "plugin-bin")

	// Even without an allowlist, paths containing traversal segments are refused.
	sneaky := dir + "/subdir/../plugin-bin"
	_, err := validatePluginExecutablePath(sneaky)
	assert.Error(t, err)
}
