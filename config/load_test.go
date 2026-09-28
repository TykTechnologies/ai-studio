package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsEnvFileWithoutExportingIt(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "test.env")
	if err := os.WriteFile(envFile, []byte("LOAD_TEST_ONLY_IN_FILE=from-file\nSERVER_PORT=7000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVER_PORT", "7001")
	os.Unsetenv("LOAD_TEST_ONLY_IN_FILE")

	conf := Load(envFile)

	if conf.ServerPort != "7001" {
		t.Errorf("environment should take precedence over the file: got ServerPort %q", conf.ServerPort)
	}
	if _, set := os.LookupEnv("LOAD_TEST_ONLY_IN_FILE"); set {
		t.Error("Load wrote a file variable into the process environment")
	}
}

func TestLoadFromUsesOnlyTheLookup(t *testing.T) {
	t.Setenv("SERVER_PORT", "7001")
	vals := map[string]string{"SITE_URL": "https://studio.example.com", "DATABASE_TYPE": "postgres"}

	conf := LoadFrom(func(k string) string { return vals[k] })

	if conf.SiteURL != "https://studio.example.com" || conf.DatabaseType != "postgres" {
		t.Errorf("lookup values not applied: SiteURL %q DatabaseType %q", conf.SiteURL, conf.DatabaseType)
	}
	if conf.ServerPort != "8080" {
		t.Errorf("expected default ServerPort 8080 ignoring the environment, got %q", conf.ServerPort)
	}
}

func TestSetOverridesGet(t *testing.T) {
	t.Cleanup(ResetGlobalConfig)
	conf := LoadFrom(func(string) string { return "" })
	conf.SiteURL = "https://host.example.com/ai-studio"

	Set(conf)

	if got := Get(""); got != conf {
		t.Fatalf("Get returned %p, want the configuration passed to Set (%p)", got, conf)
	}
}

func TestNormalizeBasePath(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "/": "", " / ": "", "ai-studio": "/ai-studio", "/ai-studio/": "/ai-studio", "/a/b/": "/a/b",
	} {
		if got := NormalizeBasePath(in); got != want {
			t.Errorf("NormalizeBasePath(%q) = %q, want %q", in, got, want)
		}
	}
	conf := LoadFrom(func(k string) string { return map[string]string{"BASE_PATH": "ai-studio/"}[k] })
	if conf.BasePath != "/ai-studio" || conf.PublicPath("/admin") != "/ai-studio/admin" {
		t.Errorf("BasePath %q, PublicPath %q", conf.BasePath, conf.PublicPath("/admin"))
	}
}
