package services

import (
	"fmt"
	"os"
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
// before it is handed to exec.Command. It returns the validated path or an
// error when the path is empty, contains traversal segments, does not point
// at a regular executable file, or (when AI_STUDIO_PLUGIN_ALLOWED_DIRS is
// set) resolves outside the allowlisted plugin directories, including via
// symlinks.
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

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve plugin executable path %q: %w", path, err)
	}

	info, err := os.Stat(abs)
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
			Str("path", abs).
			Msgf("%s not set — plugin executable location is not scope-restricted", pluginAllowedDirsEnv)
		return path, nil
	}

	// Resolve symlinks so a link inside an allowed directory cannot point at
	// a binary outside it.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks for plugin executable %q: %w", path, err)
	}

	for _, dir := range filepath.SplitList(allowedDirs) {
		if dir == "" {
			continue
		}
		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		dirResolved, err := filepath.EvalSymlinks(dirAbs)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dirResolved, resolved)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return path, nil
		}
	}

	return "", fmt.Errorf("plugin executable %q is outside the allowed plugin directories (%s=%s)", path, pluginAllowedDirsEnv, allowedDirs)
}
