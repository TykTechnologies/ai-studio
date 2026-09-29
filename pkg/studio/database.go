package studio

import (
	"fmt"
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
// it responds. The result is what Options.DB takes. A host opens Studio's
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

	db, err := gorm.Open(open(conf.DatabaseURL), logger.GetGormConfig())
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
