// internal/database/migrations/migrate.go
package migrations

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed *.sql
var migrationFiles embed.FS

// Migration represents a database migration
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// MigrationRecord tracks applied migrations
type MigrationRecord struct {
	ID        uint   `gorm:"primaryKey"`
	Version   int    `gorm:"not null;uniqueIndex"`
	Name      string `gorm:"not null"`
	AppliedAt int64  `gorm:"not null"`
}

// TableName returns the table name for migration records
func (MigrationRecord) TableName() string {
	return "schema_migrations"
}

// Migrator handles database migrations
type Migrator struct {
	db         *gorm.DB
	migrations []Migration
}

// NewMigrator creates a new migrator instance
func NewMigrator(db *gorm.DB) (*Migrator, error) {
	migrator := &Migrator{
		db:         db,
		migrations: []Migration{},
	}

	if err := migrator.loadMigrations(); err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	if err := migrator.ensureMigrationsTable(); err != nil {
		return nil, fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	return migrator, nil
}

// loadMigrations loads all migration files from embedded filesystem
func (m *Migrator) loadMigrations() error {
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return err
	}

	migrationMap := make(map[int]*Migration)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, name, direction, err := parseMigrationFilename(entry.Name())
		if err != nil {
			return fmt.Errorf("invalid migration filename %s: %w", entry.Name(), err)
		}

		content, err := migrationFiles.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		migration, exists := migrationMap[version]
		if !exists {
			migration = &Migration{
				Version: version,
				Name:    name,
			}
			migrationMap[version] = migration
		}

		switch direction {
		case "up":
			migration.UpSQL = string(content)
		case "down":
			migration.DownSQL = string(content)
		}
	}

	// Convert map to sorted slice
	versions := make([]int, 0, len(migrationMap))
	for version := range migrationMap {
		versions = append(versions, version)
	}
	sort.Ints(versions)

	for _, version := range versions {
		migration := migrationMap[version]
		if migration.UpSQL == "" {
			return fmt.Errorf("missing up migration for version %d", version)
		}
		if migration.DownSQL == "" {
			return fmt.Errorf("missing down migration for version %d", version)
		}
		m.migrations = append(m.migrations, *migration)
	}

	return nil
}

// parseMigrationFilename parses migration filename to extract version, name, and direction
// Expected format: {version}_{name}.{direction}.sql
func parseMigrationFilename(filename string) (int, string, string, error) {
	// Remove .sql extension
	name := strings.TrimSuffix(filename, ".sql")
	
	// Split by dots to get direction
	parts := strings.Split(name, ".")
	if len(parts) != 2 {
		return 0, "", "", fmt.Errorf("invalid filename format, expected: version_name.direction.sql")
	}
	
	direction := parts[1]
	if direction != "up" && direction != "down" {
		return 0, "", "", fmt.Errorf("invalid direction: %s, expected 'up' or 'down'", direction)
	}
	
	// Split by underscore to get version and name
	versionAndName := parts[0]
	underscoreParts := strings.SplitN(versionAndName, "_", 2)
	if len(underscoreParts) < 2 {
		return 0, "", "", fmt.Errorf("invalid filename format, expected: version_name.direction.sql")
	}
	
	version, err := strconv.Atoi(underscoreParts[0])
	if err != nil {
		return 0, "", "", fmt.Errorf("invalid version number: %s", underscoreParts[0])
	}
	
	migrationName := underscoreParts[1]
	
	return version, migrationName, direction, nil
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist
func (m *Migrator) ensureMigrationsTable() error {
	return m.db.AutoMigrate(&MigrationRecord{})
}

// Up runs all pending migrations
func (m *Migrator) Up() error {
	appliedVersions, err := m.getAppliedVersions()
	if err != nil {
		return fmt.Errorf("failed to get applied versions: %w", err)
	}

	for _, migration := range m.migrations {
		if appliedVersions[migration.Version] {
			continue
		}

		fmt.Printf("Applying migration %d: %s\n", migration.Version, migration.Name)
		
		err := m.db.Transaction(func(tx *gorm.DB) error {
			// Execute migration
			if err := tx.Exec(migration.UpSQL).Error; err != nil {
				return fmt.Errorf("failed to execute migration %d: %w", migration.Version, err)
			}

			// Record migration
			record := MigrationRecord{
				Version:   migration.Version,
				Name:      migration.Name,
				AppliedAt: time.Now().Unix(),
			}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
			}

			return nil
		})

		if err != nil {
			return err
		}

		fmt.Printf("Applied migration %d: %s\n", migration.Version, migration.Name)
	}

	return nil
}

// Down rolls back the last migration
func (m *Migrator) Down() error {
	// Get the latest applied migration
	var record MigrationRecord
	err := m.db.Order("version DESC").First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			fmt.Println("No migrations to roll back")
			return nil
		}
		return fmt.Errorf("failed to get latest migration: %w", err)
	}

	// Find the migration
	var migration *Migration
	for _, m := range m.migrations {
		if m.Version == record.Version {
			migration = &m
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration %d not found", record.Version)
	}

	fmt.Printf("Rolling back migration %d: %s\n", migration.Version, migration.Name)

	err = m.db.Transaction(func(tx *gorm.DB) error {
		// Execute rollback
		if err := tx.Exec(migration.DownSQL).Error; err != nil {
			return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
		}

		// Remove migration record
		if err := tx.Delete(&record).Error; err != nil {
			return fmt.Errorf("failed to remove migration record %d: %w", migration.Version, err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("Rolled back migration %d: %s\n", migration.Version, migration.Name)
	return nil
}

// Status shows the current migration status
func (m *Migrator) Status() error {
	appliedVersions, err := m.getAppliedVersions()
	if err != nil {
		return fmt.Errorf("failed to get applied versions: %w", err)
	}

	fmt.Println("Migration Status:")
	fmt.Println("=================")

	for _, migration := range m.migrations {
		status := "Pending"
		if appliedVersions[migration.Version] {
			status = "Applied"
		}
		fmt.Printf("Version %d: %s [%s]\n", migration.Version, migration.Name, status)
	}

	return nil
}

// getAppliedVersions returns a map of applied migration versions
func (m *Migrator) getAppliedVersions() (map[int]bool, error) {
	var records []MigrationRecord
	if err := m.db.Find(&records).Error; err != nil {
		return nil, err
	}

	versions := make(map[int]bool)
	for _, record := range records {
		versions[record.Version] = true
	}

	return versions, nil
}