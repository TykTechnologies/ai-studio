package config

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

func TestPluginAllowedSources(t *testing.T) {
	load := func(value string) *AppConf {
		vals := map[string]string{"AI_STUDIO_PLUGIN_ALLOWED_SOURCES": value}
		return LoadFrom(func(k string) string { return vals[k] })
	}

	conf := load("")
	if err := conf.Err(); err != nil {
		t.Fatal(err)
	}
	if !conf.PluginSourcePolicy.AllowsAll() {
		t.Error("unset must allow every source")
	}

	conf = load(" OCI, grpc ")
	if err := conf.Err(); err != nil {
		t.Fatal(err)
	}
	if err := conf.PluginSourcePolicy.Check("/opt/plugins/demo"); err == nil || !strings.Contains(err.Error(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES") {
		t.Errorf("file plugin check = %v, want a refusal naming the setting", err)
	}
	if got := conf.PluginSourcePolicy.Allowed(); len(got) != 2 || got[0] != plugin_security.SourceOCI || got[1] != plugin_security.SourceGRPC {
		t.Errorf("allowed = %v, want [oci grpc]", got)
	}

	for _, bad := range []string{"ocii", ","} {
		if err := load(bad).Err(); err == nil || !strings.Contains(err.Error(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES") {
			t.Errorf("%q: error %v does not name AI_STUDIO_PLUGIN_ALLOWED_SOURCES", bad, err)
		}
	}
}
