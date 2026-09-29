package studio

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
)

// databaseDrivers maps a DatabaseType to the function that makes its gorm
// dialector from a DSN. Postgres is built in. SQLite is not: its driver needs
// cgo, and a host that builds with CGO_ENABLED=0 must not link it, so it
// registers itself from package pkg/studio/sqlitedb, which the standalone
// binary imports.
var (
	databaseDriversMu sync.RWMutex
	databaseDrivers   = map[string]func(dsn string) gorm.Dialector{
		"postgres": postgres.Open,
	}
)

// RegisterDatabaseDriver makes OpenDatabase accept DatabaseType name, opening
// it with open. Call it from an init function, as pkg/studio/sqlitedb does.
func RegisterDatabaseDriver(name string, open func(dsn string) gorm.Dialector) {
	databaseDriversMu.Lock()
	defer databaseDriversMu.Unlock()
	databaseDrivers[name] = open
}

// OpenDatabase connects to the database conf names (DatabaseType "postgres",
// or "sqlite" once pkg/studio/sqlitedb is imported, at DatabaseURL) and checks
// it responds. With DatabaseSchema set (postgres only) Studio's tables live
// in that schema: OpenDatabase creates it when missing and pins every
// connection's search_path to it alone. The result is what Options.DB takes. A host opens Studio's
// database with it rather than with gorm itself: Studio builds with its own
// copy of gorm (third_party/gorm.io), which the host's gorm cannot stand in
// for. The caller closes it after Stop, through DB().
func OpenDatabase(conf *config.AppConf) (*gorm.DB, error) {
	databaseDriversMu.RLock()
	open, ok := databaseDrivers[conf.DatabaseType]
	databaseDriversMu.RUnlock()
	if !ok {
		if conf.DatabaseType == "sqlite" {
			return nil, fmt.Errorf("studio: database type %q needs the SQLite driver: import github.com/TykTechnologies/midsommar/v2/pkg/studio/sqlitedb (it needs cgo)", conf.DatabaseType)
		}
		return nil, fmt.Errorf("studio: unsupported database type: %q (supported: %s)", conf.DatabaseType, registeredDatabaseTypes())
	}

	dsn := conf.DatabaseURL
	if conf.DatabaseSchema != "" {
		if conf.DatabaseType != "postgres" {
			return nil, fmt.Errorf("studio: DATABASE_SCHEMA is for postgres only (DatabaseType is %q)", conf.DatabaseType)
		}
		var err error
		if dsn, err = withSchema(open, dsn, conf.DatabaseSchema); err != nil {
			return nil, err
		}
	}

	db, err := gorm.Open(open(dsn), logger.GetGormConfig())
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func registeredDatabaseTypes() string {
	databaseDriversMu.RLock()
	defer databaseDriversMu.RUnlock()
	names := make([]string, 0, len(databaseDrivers))
	for name := range databaseDrivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// schemaName is what DatabaseSchema accepts: an unquoted Postgres identifier
// in lower case, so it means the same quoted and unquoted.
var schemaName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// withSchema creates schema in the database dsn names when it does not
// exist yet, and returns dsn with search_path set to that schema alone.
// Only that schema: Postgres skips search_path entries that do not exist,
// and gorm's migrator creates tables in current_schema(), so a fallback such
// as "studio,public" would put tables in public whenever the schema is
// missing.
func withSchema(open func(string) gorm.Dialector, dsn, schema string) (string, error) {
	if !schemaName.MatchString(schema) {
		return "", fmt.Errorf("studio: DATABASE_SCHEMA %q must be a lower-case identifier (letters, digits, underscores; at most 63)", schema)
	}
	scoped, err := dsnWithSearchPath(dsn, schema)
	if err != nil {
		return "", err
	}

	admin, err := gorm.Open(open(dsn), logger.GetGormConfig())
	if err != nil {
		return "", err
	}
	if sqlDB, err := admin.DB(); err == nil {
		defer sqlDB.Close()
	}
	var exists bool
	if err := admin.Raw("SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = ?)", schema).Scan(&exists).Error; err != nil {
		return "", fmt.Errorf("studio: look up schema %q: %w", schema, err)
	}
	if !exists {
		// IF NOT EXISTS: replicas starting together may both get here.
		if err := admin.Exec(`CREATE SCHEMA IF NOT EXISTS "` + schema + `"`).Error; err != nil {
			return "", fmt.Errorf("studio: create schema %q: %w", schema, err)
		}
		logger.Infof("Created database schema %q", schema)
	}
	return scoped, nil
}

// dsnWithSearchPath sets search_path in a Postgres DSN, either a URL
// (postgres://...) or keyword/value pairs. pgx sends it as a run-time
// parameter on every connection. A DSN that already sets a different
// search_path is an error rather than silently overridden.
func dsnWithSearchPath(dsn, schema string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("studio: parse DATABASE_URL: %w", err)
		}
		q := u.Query()
		if cur := q.Get("search_path"); cur != "" && cur != schema {
			return "", fmt.Errorf("studio: DATABASE_URL sets search_path=%s but DATABASE_SCHEMA is %s", cur, schema)
		}
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	for _, field := range strings.Fields(dsn) {
		if k, v, ok := strings.Cut(field, "="); ok && k == "search_path" {
			if strings.Trim(v, "'") != schema {
				return "", fmt.Errorf("studio: DATABASE_URL sets search_path=%s but DATABASE_SCHEMA is %s", v, schema)
			}
			return dsn, nil
		}
	}
	return strings.TrimSpace(dsn) + " search_path=" + schema, nil
}
