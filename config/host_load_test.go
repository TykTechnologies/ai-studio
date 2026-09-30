package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// captureConfigLog sends the configuration logger to a buffer for the test.
func captureConfigLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := cfgLog
	cfgLog = zerolog.New(&buf)
	t.Cleanup(func() { cfgLog = prev })
	return &buf
}

// A host supplies only the settings it needs through LoadFrom; the
// standalone binary's "environment variable is not set" notices are noise
// there. Problems with the values supplied are still reported.
func TestLoadFromDoesNotLogUnsetVariables(t *testing.T) {
	buf := captureConfigLog(t)

	LoadFrom(func(k string) string {
		return map[string]string{"SMTP_PORT": "not-a-number"}[k]
	})

	out := buf.String()
	if strings.Contains(out, "not set") {
		t.Errorf("LoadFrom logged unset-variable notices:\n%s", out)
	}
	if !strings.Contains(out, "Invalid SMTP_PORT") {
		t.Errorf("an invalid supplied value should still be reported, got:\n%s", out)
	}
}

// The standalone binary (Load, Get) keeps them.
func TestLoadLogsUnsetVariables(t *testing.T) {
	buf := captureConfigLog(t)
	t.Setenv("SMTP_SERVER", "")
	t.Setenv("SITE_URL", "")

	Load(t.TempDir() + "/missing.env")

	out := buf.String()
	for _, want := range []string{"SMTP_SERVER environment variable is not set", "SITE_URL environment variable is not set"} {
		if !strings.Contains(out, want) {
			t.Errorf("Load did not log %q:\n%s", want, out)
		}
	}
}

// Only the standalone binary runs the documentation server, so a host's
// configuration has no docs link unless the host names one.
func TestDocsURLForHostsAndStandalone(t *testing.T) {
	t.Setenv("DOCS_URL_OVERRIDE", "")
	t.Setenv("DOCS_DISABLED", "")
	t.Setenv("DOCS_PORT", "")

	standalone := Load(t.TempDir() + "/missing.env")
	if standalone.DocsURL != "http://localhost:8989" || standalone.DocsDisabled {
		t.Errorf("standalone: DocsURL %q DocsDisabled %v, want http://localhost:8989 and enabled", standalone.DocsURL, standalone.DocsDisabled)
	}

	host := LoadFrom(func(string) string { return "" })
	if host.DocsURL != "" || !host.DocsDisabled {
		t.Errorf("host: DocsURL %q DocsDisabled %v, want no docs link", host.DocsURL, host.DocsDisabled)
	}

	withDocs := LoadFrom(func(k string) string {
		return map[string]string{"DOCS_URL_OVERRIDE": "https://docs.example.com"}[k]
	})
	if withDocs.DocsURL != "https://docs.example.com" || withDocs.DocsDisabled {
		t.Errorf("host with DOCS_URL_OVERRIDE: DocsURL %q DocsDisabled %v", withDocs.DocsURL, withDocs.DocsDisabled)
	}
}
