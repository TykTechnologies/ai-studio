package vendorconformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

func clearGuardrailEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"VENDOR_TESTS_ENV_FILE", "VENDOR_TESTS_GUARDRAILS", "VENDOR_TESTS_ARTIFACT_DIR",
		"VT_GUARD_LAKERA_API_KEY", "VT_GUARD_LAKERA_ENDPOINT", "VT_GUARD_LAKERA_PROJECT_ID",
		"VT_GUARD_AZURE_CS_ENDPOINT", "VT_GUARD_AZURE_CS_API_KEY",
		"VT_GUARD_AZURE_PII_ENDPOINT", "VT_GUARD_AZURE_PII_API_KEY",
		"VT_GUARD_PRESIDIO_ANALYZER_URL", "VT_GUARD_PRESIDIO_ANONYMIZER_URL",
		"VT_GUARD_BEDROCK_GUARDRAIL_ID", "VT_GUARD_BEDROCK_REGION", "VT_BEDROCK_REGION",
		"VT_GUARD_BEDROCK_ACCESS_KEY_ID", "VT_GUARD_BEDROCK_SECRET_ACCESS_KEY",
		"VT_BEDROCK_ACCESS_KEY_ID", "VT_BEDROCK_SECRET_ACCESS_KEY",
		"VT_GUARD_HTTP_ENDPOINT", "VT_GUARD_HTTP_API_KEY", "VT_GUARD_HTTP_DETECTORS",
	} {
		t.Setenv(k, "")
	}
	// Point the loader at an empty file so a developer's real
	// test-secrets/vendors.env cannot leak into the unit test.
	empty := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VENDOR_TESTS_ENV_FILE", empty)
}

func TestLoadGuardrails_BuiltinAlwaysConfiguredRemoteSkipped(t *testing.T) {
	clearGuardrailEnv(t)
	cfg, err := LoadGuardrails()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Key != "builtin" {
		t.Fatalf("providers = %+v, want only builtin", cfg.Providers)
	}
	if len(cfg.Skipped) != 6 {
		t.Errorf("skipped = %+v, want every remote provider skipped with a reason", cfg.Skipped)
	}
	for _, s := range cfg.Skipped {
		if s.Reason == "" {
			t.Errorf("%s skipped without a reason", s.Key)
		}
	}
	if len(cfg.Remote()) != 0 {
		t.Error("Remote() must exclude builtin")
	}
}

func TestLoadGuardrails_ProvidersFromEnv(t *testing.T) {
	clearGuardrailEnv(t)
	t.Setenv("VT_GUARD_LAKERA_API_KEY", "lk")
	t.Setenv("VT_GUARD_LAKERA_PROJECT_ID", "project-1")
	t.Setenv("VT_GUARD_AZURE_CS_ENDPOINT", "https://cs.example")
	t.Setenv("VT_GUARD_AZURE_CS_API_KEY", "ak")
	t.Setenv("VT_GUARD_AZURE_PII_ENDPOINT", "https://lang.example")
	t.Setenv("VT_GUARD_AZURE_PII_API_KEY", "pk")
	t.Setenv("VT_GUARD_PRESIDIO_ANALYZER_URL", "http://presidio:3000")
	t.Setenv("VT_GUARD_BEDROCK_GUARDRAIL_ID", "g1")
	t.Setenv("VT_BEDROCK_REGION", "us-east-1")
	t.Setenv("VT_BEDROCK_ACCESS_KEY_ID", "AKIA")
	t.Setenv("VT_BEDROCK_SECRET_ACCESS_KEY", "sk")
	t.Setenv("VT_GUARD_HTTP_ENDPOINT", "http://pg:8000/classify")
	t.Setenv("VT_GUARD_HTTP_DETECTORS", "prompt_attack,pii")

	cfg, err := LoadGuardrails()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Skipped) != 0 {
		t.Fatalf("skipped = %+v", cfg.Skipped)
	}
	if len(cfg.Providers) != 7 {
		t.Fatalf("providers = %d, want 7", len(cfg.Providers))
	}

	// Every provider's rendered config must pass the real normaliser: the
	// suite must not be able to seed a filter the admin API would reject.
	for _, p := range cfg.Providers {
		action := guardrails.ActionBlock
		if _, err := p.Config(action, nil, false); err != nil {
			t.Errorf("%s: config does not normalise: %v", p.Key, err)
		}
	}

	bedrock, _ := cfg.Provider("bedrock")
	if bedrock.Connection["region"] != "us-east-1" || bedrock.Connection["access_key_id"] != "AKIA" || bedrock.Connection["guardrail_version"] != "DRAFT" {
		t.Errorf("bedrock falls back to the vendor credentials and DRAFT: %+v", bedrock.Connection)
	}
	httpP, _ := cfg.Provider("http")
	if !httpP.Expect[GuardrailInjection] || !httpP.Expect[GuardrailPII] {
		t.Errorf("http expectations derive from its detectors: %+v", httpP.Expect)
	}
	lakera, _ := cfg.Provider("lakera")
	if lakera.KeyField != "api_key" || lakera.Connection["project_id"] != "project-1" {
		t.Errorf("lakera = %+v", lakera)
	}
}

func TestLoadGuardrails_FilterAndErrors(t *testing.T) {
	clearGuardrailEnv(t)
	t.Setenv("VT_GUARD_LAKERA_API_KEY", "lk")
	t.Setenv("VENDOR_TESTS_GUARDRAILS", "lakera")
	cfg, err := LoadGuardrails()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Key != "lakera" || len(cfg.Skipped) != 0 {
		t.Errorf("filter should select only lakera: %+v skipped %+v", cfg.Providers, cfg.Skipped)
	}

	t.Setenv("VENDOR_TESTS_GUARDRAILS", "nope")
	if _, err := LoadGuardrails(); err == nil {
		t.Error("an unknown provider in the filter must be an error")
	}

	t.Setenv("VENDOR_TESTS_GUARDRAILS", "")
	t.Setenv("VT_GUARD_BEDROCK_GUARDRAIL_ID", "g1")
	t.Setenv("VT_BEDROCK_REGION", "us-east-1")
	t.Setenv("VT_GUARD_BEDROCK_ACCESS_KEY_ID", "only-one-half")
	cfg, err = LoadGuardrails()
	if err != nil {
		t.Fatal(err)
	}
	var bedrockSkip string
	for _, s := range cfg.Skipped {
		if s.Key == "bedrock" {
			bedrockSkip = s.Reason
		}
	}
	if bedrockSkip == "" {
		t.Error("half a credential pair must skip bedrock with a reason")
	}
}
