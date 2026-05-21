package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgreSQL_ControlPayload_Migration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://tykuser:tykSecurePass123@localhost:5432/tyk_analytics?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Skipping PostgreSQL test: failed to connect to PostgreSQL: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		t.Skipf("Skipping PostgreSQL test: PostgreSQL is not reachable: %v", err)
	}

	// Clean up table if it exists
	db.Migrator().DropTable(&ControlPayload{})

	// Try to auto-migrate ControlPayload
	err = db.AutoMigrate(&ControlPayload{})
	require.NoError(t, err, "AutoMigrate of ControlPayload should succeed on PostgreSQL")
}
