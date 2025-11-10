package models

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupPostgreSQLTestDB sets up a PostgreSQL test database
// This test will be skipped if DATABASE_URL is not set
func setupPostgreSQLTestDB(t *testing.T) *gorm.DB {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL tests")
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	// Test database connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// Initialize models
	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

// TestUser_GetAccessibleDataSources_PostgreSQL tests the GetAccessibleDataSources method with PostgreSQL
// This specifically tests that DISTINCT ON works with JSON columns
func TestUser_GetAccessibleDataSources_PostgreSQL(t *testing.T) {
	db := setupPostgreSQLTestDB(t)

	// Clean up after test
	defer func() {
		// Clean up test data
		db.Exec("DELETE FROM data_catalogue_data_sources")
		db.Exec("DELETE FROM group_datacatalogues")
		db.Exec("DELETE FROM user_groups")
		db.Exec("DELETE FROM datasources WHERE name LIKE 'PG Test Datasource%'")
		db.Exec("DELETE FROM data_catalogues WHERE name LIKE 'PG Test Data Catalogue%'")
		db.Exec("DELETE FROM groups WHERE name LIKE 'PG Test Group%'")
		db.Exec("DELETE FROM users WHERE email = 'pgtest@example.com'")
	}()

	// Create a user
	user := &User{Email: "pgtest@example.com", Password: "password"}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create groups
	group1 := &Group{Name: "PG Test Group 1"}
	err = group1.Create(db)
	assert.NoError(t, err)

	group2 := &Group{Name: "PG Test Group 2"}
	err = group2.Create(db)
	assert.NoError(t, err)

	// Create data catalogues
	dataCatalogue1 := &DataCatalogue{Name: "PG Test Data Catalogue 1"}
	err = dataCatalogue1.Create(db)
	assert.NoError(t, err)

	dataCatalogue2 := &DataCatalogue{Name: "PG Test Data Catalogue 2"}
	err = dataCatalogue2.Create(db)
	assert.NoError(t, err)

	// Create datasources with metadata (to test JSON column handling in PostgreSQL)
	datasource1 := &Datasource{
		Name:   "PG Test Datasource 1",
		Active: true,
		Metadata: JSONMap{
			"key1":   "value1",
			"key2":   123,
			"nested": map[string]interface{}{"inner": "value"},
		},
	}
	err = datasource1.Create(db)
	assert.NoError(t, err)

	datasource2 := &Datasource{
		Name:   "PG Test Datasource 2",
		Active: true,
		Metadata: JSONMap{
			"key3": "value3",
			"array": []interface{}{1, 2, 3},
		},
	}
	err = datasource2.Create(db)
	assert.NoError(t, err)

	datasource3 := &Datasource{
		Name:   "PG Test Datasource 3 (Inactive)",
		Active: false,
		Metadata: JSONMap{
			"key4": "value4",
		},
	}
	err = datasource3.Create(db)
	assert.NoError(t, err)

	// Create duplicate association to test DISTINCT ON
	// This datasource is in both catalogues to ensure DISTINCT ON works
	datasource4 := &Datasource{
		Name:   "PG Test Datasource 4 (Duplicate)",
		Active: true,
		Metadata: JSONMap{
			"key5": "value5",
		},
	}
	err = datasource4.Create(db)
	assert.NoError(t, err)

	// Add user to groups
	err = group1.AddUser(db, user)
	assert.NoError(t, err)
	err = group2.AddUser(db, user)
	assert.NoError(t, err)

	// Add data catalogues to groups
	err = group1.AddDataCatalogue(db, dataCatalogue1)
	assert.NoError(t, err)
	err = group2.AddDataCatalogue(db, dataCatalogue2)
	assert.NoError(t, err)

	// Add datasources to data catalogues
	err = dataCatalogue1.AddDatasource(db, datasource1)
	assert.NoError(t, err)
	err = dataCatalogue1.AddDatasource(db, datasource4) // Add to first catalogue
	assert.NoError(t, err)
	err = dataCatalogue2.AddDatasource(db, datasource2)
	assert.NoError(t, err)
	err = dataCatalogue2.AddDatasource(db, datasource3) // Inactive datasource
	assert.NoError(t, err)
	err = dataCatalogue2.AddDatasource(db, datasource4) // Add to second catalogue (duplicate)
	assert.NoError(t, err)

	// Test: Get accessible datasources
	// This is the critical test - it should not fail with PostgreSQL JSON equality error
	accessibleDataSources, err := user.GetAccessibleDataSources(db)
	assert.NoError(t, err, "GetAccessibleDataSources should not fail with PostgreSQL JSON columns")

	// Should get 3 active datasources (1, 2, 4), with 4 appearing only once due to DISTINCT ON
	assert.Len(t, accessibleDataSources, 3, "Should get exactly 3 active datasources")

	// Verify the correct datasources were returned
	names := make([]string, len(accessibleDataSources))
	for i, ds := range accessibleDataSources {
		names[i] = ds.Name
	}
	assert.Contains(t, names, "PG Test Datasource 1")
	assert.Contains(t, names, "PG Test Datasource 2")
	assert.Contains(t, names, "PG Test Datasource 4 (Duplicate)")
	assert.NotContains(t, names, "PG Test Datasource 3 (Inactive)")

	// Verify metadata was loaded correctly (test JSON parsing)
	for _, ds := range accessibleDataSources {
		assert.NotNil(t, ds.Metadata)
		if ds.Name == "PG Test Datasource 1" {
			assert.Equal(t, "value1", ds.Metadata["key1"])
			assert.Equal(t, float64(123), ds.Metadata["key2"])
		}
	}
}

// TestUser_GetAccessibleTools_PostgreSQL tests the GetAccessibleTools method with PostgreSQL
// This specifically tests that DISTINCT ON works with JSON columns
func TestUser_GetAccessibleTools_PostgreSQL(t *testing.T) {
	db := setupPostgreSQLTestDB(t)

	// Clean up after test
	defer func() {
		db.Exec("DELETE FROM tool_catalogue_tools")
		db.Exec("DELETE FROM group_toolcatalogues")
		db.Exec("DELETE FROM user_groups")
		db.Exec("DELETE FROM tools WHERE name LIKE 'PG Test Tool%'")
		db.Exec("DELETE FROM tool_catalogues WHERE name LIKE 'PG Test Tool Catalogue%'")
		db.Exec("DELETE FROM groups WHERE name LIKE 'PG Test Group%'")
		db.Exec("DELETE FROM users WHERE email = 'pgtest-tools@example.com'")
	}()

	// Create a user
	user := &User{Email: "pgtest-tools@example.com", Password: "password"}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create groups
	group1 := &Group{Name: "PG Test Group 1"}
	err = group1.Create(db)
	assert.NoError(t, err)

	group2 := &Group{Name: "PG Test Group 2"}
	err = group2.Create(db)
	assert.NoError(t, err)

	// Create tool catalogues
	toolCatalogue1 := &ToolCatalogue{Name: "PG Test Tool Catalogue 1"}
	err = toolCatalogue1.Create(db)
	assert.NoError(t, err)

	toolCatalogue2 := &ToolCatalogue{Name: "PG Test Tool Catalogue 2"}
	err = toolCatalogue2.Create(db)
	assert.NoError(t, err)

	// Create tools with metadata (to test JSON column handling in PostgreSQL)
	tool1 := &Tool{
		Name:        "PG Test Tool 1",
		Description: "Test tool 1",
		ToolType:    ToolTypeREST,
		Metadata: JSONMap{
			"api_version": "v1",
			"timeout":     30,
			"config":      map[string]interface{}{"retries": 3},
		},
	}
	err = tool1.Create(db)
	assert.NoError(t, err)

	tool2 := &Tool{
		Name:        "PG Test Tool 2",
		Description: "Test tool 2",
		ToolType:    ToolTypeREST,
		Metadata: JSONMap{
			"api_version": "v2",
			"endpoints":   []interface{}{"/api/v1", "/api/v2"},
		},
	}
	err = tool2.Create(db)
	assert.NoError(t, err)

	// Create duplicate tool to test DISTINCT ON
	tool3 := &Tool{
		Name:        "PG Test Tool 3 (Duplicate)",
		Description: "Test tool 3",
		ToolType:    ToolTypeREST,
		Metadata: JSONMap{
			"api_version": "v3",
		},
	}
	err = tool3.Create(db)
	assert.NoError(t, err)

	// Add user to groups
	err = group1.AddUser(db, user)
	assert.NoError(t, err)
	err = group2.AddUser(db, user)
	assert.NoError(t, err)

	// Add tool catalogues to groups
	err = group1.AddToolCatalogue(db, toolCatalogue1)
	assert.NoError(t, err)
	err = group2.AddToolCatalogue(db, toolCatalogue2)
	assert.NoError(t, err)

	// Add tools to tool catalogues
	err = toolCatalogue1.AddTool(db, tool1)
	assert.NoError(t, err)
	err = toolCatalogue1.AddTool(db, tool3) // Add to first catalogue
	assert.NoError(t, err)
	err = toolCatalogue2.AddTool(db, tool2)
	assert.NoError(t, err)
	err = toolCatalogue2.AddTool(db, tool3) // Add to second catalogue (duplicate)
	assert.NoError(t, err)

	// Test: Get accessible tools
	// This is the critical test - it should not fail with PostgreSQL JSON equality error
	accessibleTools, err := user.GetAccessibleTools(db)
	assert.NoError(t, err, "GetAccessibleTools should not fail with PostgreSQL JSON columns")

	// Should get 3 tools, with tool3 appearing only once due to DISTINCT ON
	assert.Len(t, accessibleTools, 3, "Should get exactly 3 tools")

	// Verify the correct tools were returned
	names := make([]string, len(accessibleTools))
	for i, tool := range accessibleTools {
		names[i] = tool.Name
	}
	assert.Contains(t, names, "PG Test Tool 1")
	assert.Contains(t, names, "PG Test Tool 2")
	assert.Contains(t, names, "PG Test Tool 3 (Duplicate)")

	// Verify metadata was loaded correctly (test JSON parsing)
	for _, tool := range accessibleTools {
		assert.NotNil(t, tool.Metadata)
		if tool.Name == "PG Test Tool 1" {
			assert.Equal(t, "v1", tool.Metadata["api_version"])
			assert.Equal(t, float64(30), tool.Metadata["timeout"])
		}
	}
}
