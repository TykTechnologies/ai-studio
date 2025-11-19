package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginConfigSchemaTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestNewPluginConfigSchema(t *testing.T) {
	t.Run("Create new plugin config schema", func(t *testing.T) {
		schema := NewPluginConfigSchema()
		assert.NotNil(t, schema)
		assert.Equal(t, uint(0), schema.ID)
	})
}

func TestPluginConfigSchema_Create(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	t.Run("Create schema successfully", func(t *testing.T) {
		schema := &PluginConfigSchema{
			Command:    "/usr/bin/test-plugin",
			SchemaJSON: `{"type": "object", "properties": {"key": {"type": "string"}}}`,
		}

		err := schema.Create(db)
		assert.NoError(t, err)
		assert.NotZero(t, schema.ID)
		assert.NotZero(t, schema.LastFetched)
	})

	t.Run("Create duplicate command fails", func(t *testing.T) {
		schema1 := &PluginConfigSchema{
			Command:    "/usr/bin/duplicate",
			SchemaJSON: `{"type": "object"}`,
		}
		err := schema1.Create(db)
		assert.NoError(t, err)

		schema2 := &PluginConfigSchema{
			Command:    "/usr/bin/duplicate",
			SchemaJSON: `{"type": "object"}`,
		}
		err = schema2.Create(db)
		assert.Error(t, err) // Should fail due to unique constraint
	})
}

func TestPluginConfigSchema_GetByCommand(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	schema := &PluginConfigSchema{
		Command:    "/usr/bin/get-test",
		SchemaJSON: `{"test": true}`,
	}
	db.Create(schema)

	t.Run("Get existing schema by command", func(t *testing.T) {
		retrieved := &PluginConfigSchema{}
		err := retrieved.GetByCommand(db, "/usr/bin/get-test")
		assert.NoError(t, err)
		assert.Equal(t, schema.ID, retrieved.ID)
		assert.Equal(t, "/usr/bin/get-test", retrieved.Command)
		assert.Equal(t, `{"test": true}`, retrieved.SchemaJSON)
	})

	t.Run("Get non-existent schema by command", func(t *testing.T) {
		retrieved := &PluginConfigSchema{}
		err := retrieved.GetByCommand(db, "/usr/bin/nonexistent")
		assert.Error(t, err)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})
}

func TestPluginConfigSchema_Update(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	schema := &PluginConfigSchema{
		Command:    "/usr/bin/update-test",
		SchemaJSON: `{"version": "1.0"}`,
	}
	db.Create(schema)

	t.Run("Update schema successfully", func(t *testing.T) {
		schema.SchemaJSON = `{"version": "2.0"}`
		err := schema.Update(db)
		assert.NoError(t, err)

		// Verify update
		retrieved := &PluginConfigSchema{}
		retrieved.GetByCommand(db, "/usr/bin/update-test")
		assert.Equal(t, `{"version": "2.0"}`, retrieved.SchemaJSON)
		assert.NotZero(t, retrieved.LastFetched)
	})
}

func TestPluginConfigSchema_Delete(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	schema := &PluginConfigSchema{
		Command:    "/usr/bin/delete-test",
		SchemaJSON: `{"test": true}`,
	}
	db.Create(schema)

	t.Run("Delete schema successfully", func(t *testing.T) {
		err := schema.Delete(db)
		assert.NoError(t, err)

		// Verify deletion
		retrieved := &PluginConfigSchema{}
		err = retrieved.GetByCommand(db, "/usr/bin/delete-test")
		assert.Error(t, err)
	})
}

func TestPluginConfigSchema_Upsert(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	t.Run("Upsert creates new schema", func(t *testing.T) {
		schema := &PluginConfigSchema{}
		err := schema.Upsert(db, "/usr/bin/upsert-new", `{"new": true}`)
		assert.NoError(t, err)
		assert.NotZero(t, schema.ID)
		assert.Equal(t, "/usr/bin/upsert-new", schema.Command)
		assert.Equal(t, `{"new": true}`, schema.SchemaJSON)
	})

	t.Run("Upsert updates existing schema", func(t *testing.T) {
		// Create initial schema
		schema1 := &PluginConfigSchema{}
		err := schema1.Upsert(db, "/usr/bin/upsert-existing", `{"version": "1.0"}`)
		assert.NoError(t, err)
		firstID := schema1.ID

		// Upsert with same command (should update)
		schema2 := &PluginConfigSchema{}
		err = schema2.Upsert(db, "/usr/bin/upsert-existing", `{"version": "2.0"}`)
		assert.NoError(t, err)
		assert.Equal(t, firstID, schema2.ID) // Should be same ID
		assert.Equal(t, `{"version": "2.0"}`, schema2.SchemaJSON)
	})
}

func TestPluginConfigSchema_IsStale(t *testing.T) {
	t.Run("Fresh schema is not stale", func(t *testing.T) {
		schema := &PluginConfigSchema{
			LastFetched: time.Now(),
		}
		assert.False(t, schema.IsStale(1*time.Hour))
	})

	t.Run("Old schema is stale", func(t *testing.T) {
		schema := &PluginConfigSchema{
			LastFetched: time.Now().Add(-2 * time.Hour),
		}
		assert.True(t, schema.IsStale(1*time.Hour))
	})

	t.Run("Schema just past threshold", func(t *testing.T) {
		schema := &PluginConfigSchema{
			LastFetched: time.Now().Add(-1*time.Hour - 1*time.Second),
		}
		// Should be stale when past threshold
		assert.True(t, schema.IsStale(1*time.Hour))
	})
}

func TestPluginConfigSchemas_ListAll(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	// Create multiple schemas
	for i := 1; i <= 5; i++ {
		schema := &PluginConfigSchema{
			Command:    "/usr/bin/list-" + string(rune('0'+i)),
			SchemaJSON: `{"index": ` + string(rune('0'+i)) + `}`,
		}
		db.Create(schema)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	t.Run("List all schemas", func(t *testing.T) {
		var schemas PluginConfigSchemas
		err := schemas.ListAll(db)
		assert.NoError(t, err)
		assert.Len(t, schemas, 5)
	})

	t.Run("List all returns empty when no schemas", func(t *testing.T) {
		db2 := setupPluginConfigSchemaTest(t)
		var schemas PluginConfigSchemas
		err := schemas.ListAll(db2)
		assert.NoError(t, err)
		assert.Len(t, schemas, 0)
	})
}

func TestPluginConfigSchemas_ListByCommands(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	// Create schemas
	commands := []string{"/bin/cmd1", "/bin/cmd2", "/bin/cmd3"}
	for _, cmd := range commands {
		schema := &PluginConfigSchema{
			Command:    cmd,
			SchemaJSON: `{}`,
		}
		db.Create(schema)
	}

	t.Run("List schemas by specific commands", func(t *testing.T) {
		var schemas PluginConfigSchemas
		err := schemas.ListByCommands(db, []string{"/bin/cmd1", "/bin/cmd3"})
		assert.NoError(t, err)
		assert.Len(t, schemas, 2)
	})

	t.Run("List with empty command list", func(t *testing.T) {
		var schemas PluginConfigSchemas
		err := schemas.ListByCommands(db, []string{})
		assert.NoError(t, err)
		assert.Len(t, schemas, 0)
	})

	t.Run("List with non-existent commands", func(t *testing.T) {
		var schemas PluginConfigSchemas
		err := schemas.ListByCommands(db, []string{"/bin/nonexistent"})
		assert.NoError(t, err)
		assert.Len(t, schemas, 0)
	})
}

func TestDeleteExpiredSchemas(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	// Create old schemas
	oldSchema := &PluginConfigSchema{
		Command:     "/bin/old",
		SchemaJSON:  `{}`,
		LastFetched: time.Now().Add(-48 * time.Hour),
	}
	db.Create(oldSchema)

	// Create fresh schema
	freshSchema := &PluginConfigSchema{
		Command:     "/bin/fresh",
		SchemaJSON:  `{}`,
		LastFetched: time.Now(),
	}
	db.Create(freshSchema)

	t.Run("Delete expired schemas", func(t *testing.T) {
		deleted, err := DeleteExpiredSchemas(db, 24*time.Hour)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		// Verify old schema is gone
		retrieved := &PluginConfigSchema{}
		err = retrieved.GetByCommand(db, "/bin/old")
		assert.Error(t, err)

		// Verify fresh schema still exists
		err = retrieved.GetByCommand(db, "/bin/fresh")
		assert.NoError(t, err)
	})

	t.Run("Delete with no expired schemas", func(t *testing.T) {
		db2 := setupPluginConfigSchemaTest(t)
		schema := &PluginConfigSchema{
			Command:     "/bin/fresh-only",
			SchemaJSON:  `{}`,
			LastFetched: time.Now(),
		}
		db2.Create(schema)

		deleted, err := DeleteExpiredSchemas(db2, 1*time.Hour)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
	})
}

func TestCountSchemas(t *testing.T) {
	db := setupPluginConfigSchemaTest(t)

	t.Run("Count with no schemas", func(t *testing.T) {
		count, err := CountSchemas(db)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Count with multiple schemas", func(t *testing.T) {
		for i := 1; i <= 5; i++ {
			schema := &PluginConfigSchema{
				Command:    "/bin/count-" + string(rune('0'+i)),
				SchemaJSON: `{}`,
			}
			db.Create(schema)
		}

		count, err := CountSchemas(db)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), count)
	})
}
