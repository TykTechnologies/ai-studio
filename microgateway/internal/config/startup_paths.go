package config

import (
	"github.com/TykTechnologies/midsommar/v2/pkg/pathcheck"
)

// Defaults of the path settings, as declared in the struct tags. Kept here so
// the startup report can tell a default from a set value.
const (
	defaultEnvFile        = ".env"
	defaultDatabaseDSN    = "file:./data/microgateway.db?mode=rwc"
	defaultOCIPluginCache = "/var/lib/microgateway/plugins"
)

// StartupPaths lists the filesystem paths this configuration uses, for the
// startup path report (pkg/pathcheck). envFile is the -env flag value. Paths
// that only matter in another mode, or with a feature off, are left out.
func StartupPaths(c *Config, envFile string) []pathcheck.Entry {
	envSource := pathcheck.SourceSet
	if envFile == defaultEnvFile {
		envSource = pathcheck.SourceDefault
	}
	entries := []pathcheck.Entry{{
		Name:    "-env",
		Value:   envFile,
		Default: defaultEnvFile,
		Kind:    pathcheck.File,
		Source:  envSource,
		Use:     "environment file; when absent only the process environment is used",
	}}

	if c.Database.Type == "sqlite" {
		entries = append(entries, pathcheck.Entry{
			Name:     "DATABASE_DSN",
			Value:    c.Database.DSN,
			Default:  defaultDatabaseDSN,
			Kind:     pathcheck.SQLite,
			Required: true,
			Use:      "gateway database (configuration, local analytics, budgets)",
		})
	}

	// In edge mode the analytics pulse to the control plane is a data
	// collection plugin loaded from this file. When the file is missing the
	// loader loads no plugins and says nothing, so the edge serves traffic
	// but never reports analytics.
	pluginsUse := "data collection plugins"
	pluginsRequired := false
	if c.IsEdge() {
		pluginsUse = "data collection plugins, including the analytics pulse to the control plane"
		pluginsRequired = c.Plugins.ConfigServiceURL == ""
	}
	entries = append(entries, pathcheck.Entry{
		Name:     "PLUGINS_CONFIG_PATH",
		Value:    c.Plugins.ConfigPath,
		Kind:     pathcheck.File,
		Required: pluginsRequired,
		Use:      pluginsUse,
	})

	entries = append(entries, pathcheck.Entry{
		Name:    "OCI_PLUGINS_CACHE_DIR",
		Value:   c.OCIPlugins.CacheDir,
		Default: defaultOCIPluginCache,
		Kind:    pathcheck.Dir,
		Use:     "OCI plugin download cache",
	})

	if c.Server.TLSEnabled {
		entries = append(entries,
			pathcheck.Entry{Name: "TLS_CERT_PATH", Value: c.Server.TLSCertPath, Kind: pathcheck.File, Required: true, Use: "HTTPS certificate (TLS_ENABLED)"},
			pathcheck.Entry{Name: "TLS_KEY_PATH", Value: c.Server.TLSKeyPath, Kind: pathcheck.File, Required: true, Use: "HTTPS private key (TLS_ENABLED)"},
		)
	}

	if c.IsControl() && c.HubSpoke.TLSEnabled {
		entries = append(entries,
			pathcheck.Entry{Name: "GRPC_TLS_CERT_PATH", Value: c.HubSpoke.TLSCertPath, Kind: pathcheck.File, Required: true, Use: "control gRPC certificate (GRPC_TLS_ENABLED)"},
			pathcheck.Entry{Name: "GRPC_TLS_KEY_PATH", Value: c.HubSpoke.TLSKeyPath, Kind: pathcheck.File, Required: true, Use: "control gRPC private key (GRPC_TLS_ENABLED)"},
		)
	}

	if c.IsEdge() && c.HubSpoke.ClientTLSEnabled {
		entries = append(entries,
			pathcheck.Entry{Name: "EDGE_TLS_CA_PATH", Value: c.HubSpoke.ClientTLSCAPath, Kind: pathcheck.File, Use: "CA for the control plane certificate; unset uses the system roots"},
			pathcheck.Entry{Name: "EDGE_TLS_CERT_PATH", Value: c.HubSpoke.ClientTLSCertPath, Kind: pathcheck.File, Use: "client certificate for mutual TLS to the control plane"},
			pathcheck.Entry{Name: "EDGE_TLS_KEY_PATH", Value: c.HubSpoke.ClientTLSKeyPath, Kind: pathcheck.File, Required: c.HubSpoke.ClientTLSCertPath != "", Use: "client key for mutual TLS to the control plane"},
		)
	}

	return entries
}
