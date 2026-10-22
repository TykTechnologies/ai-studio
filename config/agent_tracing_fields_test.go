package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// reservedAgentTracingFields are the AgentTracingConfig fields behind
// AgentTracingReservedEnv: parsed, not used yet, warned about when set.
var reservedAgentTracingFields = map[string]string{
	"PayloadBackend":            "AGENT_TRACING_PAYLOAD_BACKEND",
	"RetryWindow":               "AGENT_TRACING_RETRY_WINDOW",
	"DelegationToolPatterns":    "AGENT_TRACING_DELEGATION_TOOL_PATTERNS",
	"ExportMaxSpanContentBytes": "AGENT_TRACING_EXPORT_MAX_SPAN_CONTENT_BYTES",
}

// Every AGENT_TRACING_* setting does something: each AgentTracingConfig
// field is read outside the config package's own parsing, or is on the
// reserved list, which warns when set (jyx.110). The fields are read in both
// the core and the Enterprise module, so the test needs the enterprise
// submodule checked out.
func TestAgentTracingConfigFieldsAreRead(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "enterprise", "features", "agenttracing")); err != nil {
		t.Skip("needs the enterprise submodule, where most settings are read")
	}

	var src strings.Builder
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "third_party", ".git", "ui", "docs", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			path == filepath.Join(root, "config", "agent_tracing.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src.Write(b)
		src.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	all := src.String()

	reserved := map[string]bool{}
	for _, k := range AgentTracingReservedEnv {
		reserved[k] = true
	}
	typ := reflect.TypeOf(AgentTracingConfig{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if env, ok := reservedAgentTracingFields[name]; ok {
			if !reserved[env] {
				t.Errorf("%s is listed as reserved but %s is not in AgentTracingReservedEnv", name, env)
			}
			if regexp.MustCompile(`\.` + name + `\b`).MatchString(all) {
				t.Errorf("%s is read now: take it off the reserved lists", name)
			}
			continue
		}
		if !regexp.MustCompile(`\.` + name + `\b`).MatchString(all) {
			t.Errorf("AgentTracingConfig.%s is parsed but never read: wire it up or reserve it (AgentTracingReservedEnv)", name)
		}
	}
}

// A reserved setting warns when set; PAYLOAD_BACKEND only when it asks for
// something other than the database.
func TestAgentTracingReservedSettingsWarn(t *testing.T) {
	env := map[string]string{
		"AGENT_TRACING_RETRY_WINDOW":                  "10s",
		"AGENT_TRACING_EXPORT_MAX_SPAN_CONTENT_BYTES": "1024",
		"AGENT_TRACING_PAYLOAD_BACKEND":               "db",
	}
	_, warnings := parseAgentTracingConfig(func(k string) string { return env[k] })
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"AGENT_TRACING_RETRY_WINDOW", "AGENT_TRACING_EXPORT_MAX_SPAN_CONTENT_BYTES"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no warning for %s: %v", want, warnings)
		}
	}
	if strings.Contains(joined, "AGENT_TRACING_PAYLOAD_BACKEND") {
		t.Errorf("PAYLOAD_BACKEND=db is the built backend and must not warn: %v", warnings)
	}
	env["AGENT_TRACING_PAYLOAD_BACKEND"] = "filesystem"
	_, warnings = parseAgentTracingConfig(func(k string) string { return env[k] })
	if !strings.Contains(strings.Join(warnings, "\n"), "AGENT_TRACING_PAYLOAD_BACKEND") {
		t.Errorf("PAYLOAD_BACKEND=filesystem must warn: %v", warnings)
	}
}
