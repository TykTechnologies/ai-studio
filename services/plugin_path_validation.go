package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// pluginAllowedDirsEnv lists directories (separated by os.PathListSeparator)
// that local plugin executables may be launched from. When set, any plugin
// command resolving outside these directories is refused. When unset, plugins
// may run from any location (backwards compatible), but paths are still
// checked for traversal sequences and basic executable sanity.
const pluginAllowedDirsEnv = "AI_STUDIO_PLUGIN_ALLOWED_DIRS"

// validatePluginExecutablePath validates a local plugin executable path
// before it is handed to exec.Command, and returns the fully resolved
// absolute path that was validated. Callers MUST execute the returned path —
// not the original input — so that the binary that runs is exactly the one
// that was checked (no PATH-vs-cwd or symlink discrepancy).
//
// Bare command names (no path separator) are resolved through PATH via
// exec.LookPath, matching exec.Command semantics. Symlinks are always
// resolved before validation. It returns an error when the path is empty,
// contains traversal segments, does not point at a regular executable file,
// or (when AI_STUDIO_PLUGIN_ALLOWED_DIRS is set) resolves outside the
// allowlisted plugin directories.
func validatePluginExecutablePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("plugin executable path is empty")
	}

	// Reject traversal sequences outright — plugin commands are stored
	// configuration and have no legitimate need for "..".
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == ".." {
			return "", fmt.Errorf("plugin executable path %q contains a path traversal segment", path)
		}
	}

	// Resolve the path the same way exec.Command would: bare names go
	// through PATH, everything else is relative to the working directory.
	abs := path
	if !strings.ContainsRune(path, os.PathSeparator) {
		found, err := exec.LookPath(path)
		if err != nil {
			return "", fmt.Errorf("plugin executable %q not found in PATH: %w", path, err)
		}
		abs = found
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", fmt.Errorf("failed to resolve plugin executable path %q: %w", path, err)
	}

	// Follow symlinks so the path we validate is the file that will run.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks for plugin executable %q: %w", path, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("plugin executable %q is not accessible: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("plugin executable %q is not a regular file", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("plugin executable %q is not executable", path)
	}

	allowedDirs := os.Getenv(pluginAllowedDirsEnv)
	if allowedDirs == "" {
		log.Debug().
			Str("path", resolved).
			Msgf("%s not set — plugin executable location is not scope-restricted", pluginAllowedDirsEnv)
		return resolved, nil
	}

	for _, dir := range filepath.SplitList(allowedDirs) {
		if dir == "" {
			continue
		}
		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			log.Warn().Err(err).Str("dir", dir).Msgf("ignoring unresolvable entry in %s", pluginAllowedDirsEnv)
			continue
		}
		dirResolved, err := filepath.EvalSymlinks(dirAbs)
		if err != nil {
			log.Warn().Err(err).Str("dir", dir).Msgf("ignoring inaccessible entry in %s", pluginAllowedDirsEnv)
			continue
		}
		rel, err := filepath.Rel(dirResolved, resolved)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("plugin executable %q is outside the allowed plugin directories (%s=%s)", path, pluginAllowedDirsEnv, allowedDirs)
}
