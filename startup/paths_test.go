package startup

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/pathcheck"
)

func pathsByName(entries []pathcheck.Entry) map[string]pathcheck.Entry {
	out := map[string]pathcheck.Entry{}
	for _, e := range entries {
		out[e.Name] = e
	}
	return out
}

func TestPaths(t *testing.T) {
	t.Setenv("BRANDING_STORAGE_PATH", "")
	t.Setenv("EXPORT_STORAGE_PATH", "/srv/exports")

	t.Run("sqlite with defaults", func(t *testing.T) {
		// gRPC TLS is on by default, but the control server does not run
		// outside control mode, so its key pair is not listed.
		cfg := &config.AppConf{DatabaseType: "sqlite", DatabaseURL: "midsommar.db", GRPCTLSEnabled: true}
		got := pathsByName(Paths(cfg, ""))

		if env := got["-env"]; env.Value != ".env" || env.Source != pathcheck.SourceDefault {
			t.Errorf("-env = %+v", env)
		}
		if db := got["DATABASE_URL"]; db.Kind != pathcheck.SQLite || !db.Required {
			t.Errorf("DATABASE_URL = %+v", db)
		}
		if b := got["BRANDING_STORAGE_PATH"]; b.Value != defaultBrandingStorage {
			t.Errorf("BRANDING_STORAGE_PATH = %q, want the default", b.Value)
		}
		if e := got["EXPORT_STORAGE_PATH"]; e.Value != "/srv/exports" {
			t.Errorf("EXPORT_STORAGE_PATH = %q, want the env value", e.Value)
		}
		if oci, ok := got["AI_STUDIO_OCI_CACHE_DIR"]; !ok || oci.Required {
			t.Errorf("AI_STUDIO_OCI_CACHE_DIR = %+v, ok=%v; want an optional entry", oci, ok)
		}
		for _, name := range []string{"CERT_FILE", "GRPC_TLS_CERT_PATH", "MARKETPLACE_CACHE_DIR", "AUDIT_FILE_PATH"} {
			if _, ok := got[name]; ok {
				t.Errorf("%s listed with its feature off", name)
			}
		}
	})

	t.Run("postgres has no database path", func(t *testing.T) {
		cfg := &config.AppConf{DatabaseType: "postgres", DatabaseURL: "postgres://u:secret@db/x"}
		if _, ok := pathsByName(Paths(cfg, "/etc/studio.env"))["DATABASE_URL"]; ok {
			t.Error("DATABASE_URL listed for postgres")
		}
	})

	t.Run("features on list their paths", func(t *testing.T) {
		cfg := &config.AppConf{
			DatabaseType:        "sqlite",
			CertFile:            "/certs/fullchain.pem",
			GatewayMode:         "control",
			GRPCTLSEnabled:      true,
			MarketplaceEnabled:  true,
			MarketplaceCacheDir: "./.marketplace-cache",
		}
		cfg.OCIPlugins.CacheDir = "/data/cache/plugins"
		cfg.Audit.Enabled = true
		cfg.Audit.StoreType = config.AuditStoreBoth
		cfg.Audit.FilePath = "./data/audit/audit.log"
		cfg.QueueConfig.Type = "nats"
		cfg.QueueConfig.NATS.CredentialsFile = "/nats/user.creds"

		got := pathsByName(Paths(cfg, "/etc/studio.env"))
		// A certificate without a key is a mistake, so both are listed.
		for _, name := range []string{"CERT_FILE", "KEY_FILE", "GRPC_TLS_CERT_PATH", "GRPC_TLS_KEY_PATH", "AUDIT_FILE_PATH", "NATS_CREDENTIALS_FILE"} {
			if e, ok := got[name]; !ok || !e.Required {
				t.Errorf("%s = %+v, ok=%v; want required", name, e, ok)
			}
		}
		if _, ok := got["MARKETPLACE_CACHE_DIR"]; !ok {
			t.Error("MARKETPLACE_CACHE_DIR missing with OCI and the marketplace on")
		}
		if _, ok := got["NATS_NKEY_FILE"]; ok {
			t.Error("unset NATS_NKEY_FILE listed")
		}
		if got["-env"].Source != pathcheck.SourceSet {
			t.Errorf("-env source = %s, want set", got["-env"].Source)
		}
	})
}
