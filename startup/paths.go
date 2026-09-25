package startup

import (
	"os"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/pathcheck"
)

// Defaults of path settings that are read outside config.AppConf. They
// mirror services.DefaultBrandingStoragePath and the EXPORT_STORAGE_PATH
// fallback in main.go / services.NewServiceWithOCI.
const (
	defaultEnvFile          = ".env"
	defaultSQLiteDatabase   = "midsommar.db"
	defaultBrandingStorage  = "./data/branding"
	defaultExportStorage    = "./data/exports"
	defaultMarketplaceCache = "./.marketplace-cache"
	defaultAuditFile        = "./data/audit/audit.log"
)

// ReportPaths logs every filesystem path Studio is configured with, one
// "startup path" line each (see pkg/pathcheck), and returns the number of
// problems. It never stops startup.
func ReportPaths(cfg *config.AppConf, envFile string) int {
	return pathcheck.Log(logger.With().Logger(), "studio", pathcheck.Check(Paths(cfg, envFile)))
}

// Paths lists the filesystem paths cfg uses. envFile is the -env flag value
// ("" means .env in the working directory). Paths for features that are off
// are left out.
func Paths(cfg *config.AppConf, envFile string) []pathcheck.Entry {
	envEntry := pathcheck.Entry{
		Name:    "-env",
		Value:   envFile,
		Default: defaultEnvFile,
		Kind:    pathcheck.File,
		Source:  pathcheck.SourceSet,
		Use:     "environment file; when absent only the process environment is used",
	}
	if envFile == "" {
		envEntry.Value = defaultEnvFile
		envEntry.Source = pathcheck.SourceDefault
	}
	entries := []pathcheck.Entry{envEntry}

	if cfg.DatabaseType == "sqlite" {
		entries = append(entries, pathcheck.Entry{
			Name:     "DATABASE_URL",
			Value:    cfg.DatabaseURL,
			Default:  defaultSQLiteDatabase,
			Kind:     pathcheck.SQLite,
			Required: true,
			Use:      "Studio database",
		})
	}

	// HTTPS is on only when both are set; one without the other is a mistake.
	if cfg.CertFile != "" || cfg.KeyFile != "" {
		entries = append(entries,
			pathcheck.Entry{Name: "CERT_FILE", Value: cfg.CertFile, Kind: pathcheck.File, Required: true, Use: "HTTPS certificate"},
			pathcheck.Entry{Name: "KEY_FILE", Value: cfg.KeyFile, Kind: pathcheck.File, Required: true, Use: "HTTPS private key"},
		)
	}

	// The control server runs only in control mode; its TLS is on unless
	// GRPC_TLS_INSECURE is set.
	if cfg.GatewayMode == "control" && cfg.GRPCTLSEnabled {
		entries = append(entries,
			pathcheck.Entry{Name: "GRPC_TLS_CERT_PATH", Value: cfg.GRPCTLSCertPath, Kind: pathcheck.File, Required: true, Use: "control gRPC certificate (edges connect here)"},
			pathcheck.Entry{Name: "GRPC_TLS_KEY_PATH", Value: cfg.GRPCTLSKeyPath, Kind: pathcheck.File, Required: true, Use: "control gRPC private key"},
		)
	}

	entries = append(entries, pathcheck.Entry{
		Name:  "AI_STUDIO_OCI_CACHE_DIR",
		Value: cfg.OCIPlugins.CacheDir,
		Kind:  pathcheck.Dir,
		Use:   "OCI plugin cache; unset disables OCI plugins and the plugin marketplace",
	})
	if cfg.OCIPlugins.IsEnabled() && cfg.MarketplaceEnabled {
		entries = append(entries, pathcheck.Entry{
			Name:    "MARKETPLACE_CACHE_DIR",
			Value:   cfg.MarketplaceCacheDir,
			Default: defaultMarketplaceCache,
			Kind:    pathcheck.Dir,
			Use:     "plugin marketplace index cache",
		})
	}

	entries = append(entries,
		pathcheck.Entry{
			Name:    "BRANDING_STORAGE_PATH",
			Value:   envOr("BRANDING_STORAGE_PATH", defaultBrandingStorage),
			Default: defaultBrandingStorage,
			Kind:    pathcheck.Dir,
			Use:     "uploaded logo and favicon",
		},
		pathcheck.Entry{
			Name:    "EXPORT_STORAGE_PATH",
			Value:   envOr("EXPORT_STORAGE_PATH", defaultExportStorage),
			Default: defaultExportStorage,
			Kind:    pathcheck.Dir,
			Use:     "log export files",
		},
	)

	if cfg.Audit.Enabled && (cfg.Audit.StoreType == config.AuditStoreFile || cfg.Audit.StoreType == config.AuditStoreBoth) {
		entries = append(entries, pathcheck.Entry{
			Name:     "AUDIT_FILE_PATH",
			Value:    cfg.Audit.FilePath,
			Default:  defaultAuditFile,
			Kind:     pathcheck.OutputFile,
			Required: true,
			Use:      "audit trail log file (AUDIT_STORE_TYPE " + cfg.Audit.StoreType + ")",
		})
	}

	if cfg.QueueConfig.Type == "nats" {
		nats := cfg.QueueConfig.NATS
		for _, f := range []struct{ name, value, use string }{
			{"NATS_CREDENTIALS_FILE", nats.CredentialsFile, "NATS credentials"},
			{"NATS_NKEY_FILE", nats.NKeyFile, "NATS NKey seed"},
			{"NATS_TLS_CERT_FILE", nats.TLSCertFile, "NATS client certificate"},
			{"NATS_TLS_KEY_FILE", nats.TLSKeyFile, "NATS client key"},
			{"NATS_TLS_CA_FILE", nats.TLSCAFile, "NATS CA certificate"},
		} {
			if f.value != "" {
				entries = append(entries, pathcheck.Entry{Name: f.name, Value: f.value, Kind: pathcheck.File, Required: true, Use: f.use})
			}
		}
	}

	return entries
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
