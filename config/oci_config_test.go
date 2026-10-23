package config

import (
	"strings"
	"testing"
)

func TestOCIRequireSignatureParsing(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false}, // the default
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
