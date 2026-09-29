package sqlitedb_test

import (
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
	_ "github.com/TykTechnologies/midsommar/v2/pkg/studio/sqlitedb"
)

// The standalone binary opens SQLite this way: importing the package is all
// it takes for studio.OpenDatabase to accept DatabaseType "sqlite".
func TestImportEnablesSQLite(t *testing.T) {
	conf := &config.AppConf{DatabaseType: "sqlite", DatabaseURL: filepath.Join(t.TempDir(), "studio.db")}
	db, err := studio.OpenDatabase(conf)
	if err != nil {
		t.Fatalf("OpenDatabase(sqlite): %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if got := db.Dialector.Name(); got != "sqlite" {
		t.Fatalf("dialector = %q, want sqlite", got)
	}
}
