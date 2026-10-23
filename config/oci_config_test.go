package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

func loadOCIConfig(t *testing.T, vals map[string]string) *AppConf {
	t.Helper()
	vals["AI_STUDIO_OCI_CACHE_DIR"] = t.TempDir()
	return LoadFrom(func(k string) string { return vals[k] })
}

func TestOCIVerificationSettings(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "trusted_root.json")
	if err := os.WriteFile(rootFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	conf := loadOCIConfig(t, map[string]string{})
	if err := conf.Err(); err != nil {
		t.Fatal(err)
	}
	lib := conf.OCIPlugins.ToOCILibConfig()
	if lib.TlogPolicy != ociplugins.TlogPolicyRequired || lib.TrustedRootFile != "" {
		t.Errorf("defaults: TlogPolicy %q TrustedRootFile %q, want required and none", lib.TlogPolicy, lib.TrustedRootFile)
	}

	conf = loadOCIConfig(t, map[string]string{"AI_STUDIO_OCI_TLOG_POLICY": "Optional", "AI_STUDIO_OCI_TRUSTED_ROOT_FILE": rootFile})
	if err := conf.Err(); err != nil {
		t.Fatal(err)
	}
	lib = conf.OCIPlugins.ToOCILibConfig()
	if lib.TlogPolicy != ociplugins.TlogPolicyOptional || lib.TrustedRootFile != rootFile {
		t.Errorf("TlogPolicy %q TrustedRootFile %q, want optional and %s", lib.TlogPolicy, lib.TrustedRootFile, rootFile)
	}
}

func TestOCIVerificationSettingsInvalid(t *testing.T) {
	cases := map[string]map[string]string{
		"AI_STUDIO_OCI_TLOG_POLICY":       {"AI_STUDIO_OCI_TLOG_POLICY": "off"},
		"AI_STUDIO_OCI_TRUSTED_ROOT_FILE": {"AI_STUDIO_OCI_TRUSTED_ROOT_FILE": filepath.Join(t.TempDir(), "missing.json")},
	}
	for envVar, vals := range cases {
		t.Run(envVar, func(t *testing.T) {
			err := loadOCIConfig(t, vals).Err()
			if err == nil || !strings.Contains(err.Error(), envVar) {
				t.Fatalf("error %v does not name %s", err, envVar)
			}
		})
	}

	// Validate rejects the same values set programmatically.
	c := OCIConfig{CacheDir: t.TempDir(), TlogPolicy: "off"}
	c.SetDefaults()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "AI_STUDIO_OCI_TLOG_POLICY") {
		t.Errorf("Validate() = %v, want an AI_STUDIO_OCI_TLOG_POLICY error", err)
	}
}

func TestOCITlogPolicyDisabledWarnsOnce(t *testing.T) {
	var logs bytes.Buffer
	saved := cfgLog
	cfgLog = zerolog.New(&logs)
	t.Cleanup(func() { cfgLog = saved })

	loadOCIConfig(t, map[string]string{"AI_STUDIO_OCI_TLOG_POLICY": "disabled"})
	if n := strings.Count(logs.String(), "AI_STUDIO_OCI_TLOG_POLICY=disabled"); n != 1 {
		t.Errorf("got %d disabled-tlog warnings, want 1:\n%s", n, logs.String())
	}
}

func TestOCIRequireSignatureParsing(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", true}, // the default
		{"false", false},
		{"true", true},
		{"TRUE", true},
		{"0", false},
		{"1", true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			vals := map[string]string{"AI_STUDIO_OCI_CACHE_DIR": t.TempDir(), "AI_STUDIO_OCI_REQUIRE_SIGNATURE": tc.value}
			conf := LoadFrom(func(k string) string { return vals[k] })
			if err := conf.Err(); err != nil {
				t.Fatalf("unexpected config error: %v", err)
			}
			if conf.OCIPlugins.RequireSignature != tc.want {
				t.Errorf("RequireSignature = %t, want %t", conf.OCIPlugins.RequireSignature, tc.want)
			}
		})
	}
}

// A typo in a security setting refuses to start instead of silently using
// the default.
func TestOCIRequireSignatureInvalidValueIsAnError(t *testing.T) {
	for _, value := range []string{"ture", "yes", "on"} {
		t.Run(value, func(t *testing.T) {
			vals := map[string]string{"AI_STUDIO_OCI_CACHE_DIR": t.TempDir(), "AI_STUDIO_OCI_REQUIRE_SIGNATURE": value}
			conf := LoadFrom(func(k string) string { return vals[k] })
			err := conf.Err()
			if err == nil {
				t.Fatal("expected a config error")
			}
			if !strings.Contains(err.Error(), "AI_STUDIO_OCI_REQUIRE_SIGNATURE") {
				t.Errorf("error %q does not name the variable", err)
			}
		})
	}
}
