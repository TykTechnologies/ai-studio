package config

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/pathcheck"
)

func entriesByName(entries []pathcheck.Entry) map[string]pathcheck.Entry {
	out := map[string]pathcheck.Entry{}
	for _, e := range entries {
		out[e.Name] = e
	}
	return out
}

func TestStartupPaths(t *testing.T) {
	t.Run("edge without a plugin config reports the pulse config as required", func(t *testing.T) {
		c := &Config{}
		c.HubSpoke.Mode = "edge"
		c.HubSpoke.ClientTLSEnabled = true // the env default
		c.Database.Type = "sqlite"
		c.Database.DSN = "file:/opt/tyk-microgateway/data/edge.db?mode=rwc"

		got := entriesByName(StartupPaths(c, ".env"))
		plugins, ok := got["PLUGINS_CONFIG_PATH"]
		if !ok || !plugins.Required {
			t.Fatalf("PLUGINS_CONFIG_PATH = %+v, want a required entry", plugins)
		}
		if env := got["-env"]; env.Source != pathcheck.SourceDefault {
			t.Errorf("-env source = %s, want default", env.Source)
		}
		if db := got["DATABASE_DSN"]; db.Kind != pathcheck.SQLite || !db.Required {
			t.Errorf("DATABASE_DSN = %+v", db)
		}
		// With edge TLS on, the CA and client cert are optional.
		if ca, ok := got["EDGE_TLS_CA_PATH"]; !ok || ca.Required {
			t.Errorf("EDGE_TLS_CA_PATH = %+v, ok=%v", ca, ok)
		}
	})

	t.Run("a plugin config service makes the file optional", func(t *testing.T) {
		c := &Config{}
		c.HubSpoke.Mode = "edge"
		c.Plugins.ConfigServiceURL = "https://plugins.example"
		if got := entriesByName(StartupPaths(c, ".env"))["PLUGINS_CONFIG_PATH"]; got.Required {
			t.Errorf("PLUGINS_CONFIG_PATH required with a config service")
		}
	})

	t.Run("standalone postgres without TLS", func(t *testing.T) {
		c := &Config{}
		c.HubSpoke.Mode = "standalone"
		c.Database.Type = "postgres"

		got := entriesByName(StartupPaths(c, "/etc/mgw.env"))
		for _, name := range []string{"DATABASE_DSN", "TLS_CERT_PATH", "GRPC_TLS_CERT_PATH", "EDGE_TLS_CA_PATH"} {
			if _, ok := got[name]; ok {
				t.Errorf("%s listed for standalone postgres without TLS", name)
			}
		}
		if got["PLUGINS_CONFIG_PATH"].Required {
			t.Errorf("PLUGINS_CONFIG_PATH required in standalone mode")
		}
		if got["-env"].Source != pathcheck.SourceSet {
			t.Errorf("-env source = %s, want set", got["-env"].Source)
		}
	})

	t.Run("TLS and control gRPC TLS list their key pairs as required", func(t *testing.T) {
		c := &Config{}
		c.HubSpoke.Mode = "control"
		c.HubSpoke.TLSEnabled = true
		c.Server.TLSEnabled = true

		got := entriesByName(StartupPaths(c, ".env"))
		for _, name := range []string{"TLS_CERT_PATH", "TLS_KEY_PATH", "GRPC_TLS_CERT_PATH", "GRPC_TLS_KEY_PATH"} {
			if e, ok := got[name]; !ok || !e.Required {
				t.Errorf("%s = %+v, ok=%v; want required", name, e, ok)
			}
		}
	})
}
