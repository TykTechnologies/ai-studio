package studio

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
)

// OpenDatabase connects to the database conf names (DatabaseType "sqlite" or
// "postgres", at DatabaseURL) and checks it responds. The result is what
// Options.DB takes. A host opens Studio's database with it rather than with
// gorm itself: Studio builds with its own copy of gorm (third_party/gorm.io),
// which the host's gorm cannot stand in for. The caller closes it after
// Stop, through DB().
func OpenDatabase(conf *config.AppConf) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch conf.DatabaseType {
	case "sqlite":
		dialector = sqlite.Open(conf.DatabaseURL)
	case "postgres":
		dialector = postgres.Open(conf.DatabaseURL)
	default:
		return nil, fmt.Errorf("studio: unsupported database type: %q", conf.DatabaseType)
	}

	db, err := gorm.Open(dialector, logger.GetGormConfig())
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
