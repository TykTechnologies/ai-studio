package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// QueryCountLogger captures the number of database queries executed
type QueryCountLogger struct {
	QueryCount int
	Queries    []string
}

func (l *QueryCountLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

func (l *QueryCountLogger) Info(ctx context.Context, msg string, data ...interface{}) {}
func (l *QueryCountLogger) Warn(ctx context.Context, msg string, data ...interface{}) {}
func (l *QueryCountLogger) Error(ctx context.Context, msg string, data ...interface{}) {}

func (l *QueryCountLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, _ := fc()
	l.QueryCount++
	l.Queries = append(l.Queries, sql)
}

func setupN1TestDB(t *testing.T) (*gorm.DB, *QueryCountLogger) {
	queryLogger := &QueryCountLogger{}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: queryLogger,
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLM{}, &models.Plugin{}, &models.Filter{}, &models.LLMPlugin{})
	require.NoError(t, err)

	return db, queryLogger
}

func createTestDataForN1(t *testing.T, db *gorm.DB) {
	// Create test LLMs
	for i := 1; i <= 3; i++ {
		llm := &models.LLM{
			Model:        gorm.Model{ID: uint(i)},
			Name:         fmt.Sprintf("TestLLM%d", i),
			Vendor:       models.OPENAI,
			DefaultModel: fmt.Sprintf("test-model-%d", i),
			Active:       true,
			Namespace:    "",
		}
		err := db.Create(llm).Error
		require.NoError(t, err)
	}

	// Create test plugins
	for i := 1; i <= 5; i++ {
		plugin := &models.Plugin{
			Model:       gorm.Model{ID: uint(i)},
			Name:        fmt.Sprintf("TestPlugin%d", i),
			Slug:        fmt.Sprintf("test-plugin-%d", i),
			Description: fmt.Sprintf("Test plugin %d", i),
			Command:     fmt.Sprintf("./plugin%d", i),
			HookType:    "post_auth",
			IsActive:    true,
			Namespace:   "",
		}
		err := db.Create(plugin).Error
		require.NoError(t, err)
	}

	// Create test filters
	for i := 1; i <= 3; i++ {
		filter := &models.Filter{
			Model:       gorm.Model{ID: uint(i)},
			Name:        fmt.Sprintf("TestFilter%d", i),
			Script:      []byte(fmt.Sprintf("return input // filter %d", i)),
			Description: fmt.Sprintf("Test filter %d", i),
			Namespace:   "",
		}
		err := db.Create(filter).Error
		require.NoError(t, err)
	}

	// Create LLM-Plugin associations
	for llmID := 1; llmID <= 3; llmID++ {
		for pluginID := 1; pluginID <= 2; pluginID++ { // Each LLM has 2 plugins
			assoc := &models.LLMPlugin{
				LLMID:    uint(llmID),
				PluginID: uint(pluginID),
				IsActive: true,
			}
			err := db.Create(assoc).Error
			require.NoError(t, err)
		}
	}

	// Create LLM-Filter associations using GORM many2many
	for llmID := 1; llmID <= 3; llmID++ {
		var llm models.LLM
		err := db.First(&llm, llmID).Error
		require.NoError(t, err)

		var filters []models.Filter
		err = db.Where("id IN ?", []uint{1, 2}).Find(&filters).Error
		require.NoError(t, err)

		// Associate filters with LLM using GORM's many2many
		err = db.Model(&llm).Association("Filters").Append(filters)
		require.NoError(t, err)
	}
}

func TestLLMService_GetAllLLMs_PreventN1Queries(t *testing.T) {
	db, queryLogger := setupN1TestDB(t)
	createTestDataForN1(t, db)

	service := NewService(db)

	// Reset query counter
	queryLogger.QueryCount = 0

	// Call GetAllLLMs which should preload plugins and filters
	llms, totalCount, totalPages, err := service.GetAllLLMs(10, 1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(3), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Len(t, llms, 3)

	// Verify that plugins are preloaded (no additional queries needed)
	for _, llm := range llms {
		assert.Len(t, llm.Plugins, 2, "Each LLM should have 2 plugins preloaded")
		assert.Len(t, llm.Filters, 2, "Each LLM should have 2 filters preloaded")

		// Access plugins to ensure they're loaded
		for _, plugin := range llm.Plugins {
			assert.NotEmpty(t, plugin.Name)
		}

		// Access filters to ensure they're loaded
		for _, filter := range llm.Filters {
			assert.NotEmpty(t, filter.Name)
		}
	}

	// With proper preloading, we should have a minimal number of queries:
	// 1. COUNT query for pagination
	// 2. Main LLM query with preloads
	// 3. Additional queries for many2many associations
	// Total should be <10 queries, not 3 + (3*2) + (3*2) = 15 queries (N+1 pattern)
	assert.LessOrEqual(t, queryLogger.QueryCount, 10,
		"Query count should be minimal with proper preloading (got %d queries)", queryLogger.QueryCount)

	// Log the actual queries for debugging
	t.Logf("Executed %d queries:", queryLogger.QueryCount)
	for i, query := range queryLogger.Queries {
		t.Logf("Query %d: %s", i+1, query)
	}
}

func TestLLMService_GetLLMsByNameStub_PreventN1Queries(t *testing.T) {
	db, queryLogger := setupN1TestDB(t)
	createTestDataForN1(t, db)

	service := NewService(db)

	// Reset query counter
	queryLogger.QueryCount = 0

	// Call GetLLMsByNameStub which should preload plugins and filters
	llms, err := service.GetLLMsByNameStub("Test")
	require.NoError(t, err)
	assert.Len(t, llms, 3)

	// Verify that plugins are preloaded
	for _, llm := range llms {
		assert.Len(t, llm.Plugins, 2, "Each LLM should have 2 plugins preloaded")
		assert.Len(t, llm.Filters, 2, "Each LLM should have 2 filters preloaded")
	}

	// Should be ≤5 queries with proper preloading (LLMs + 2 many2many preloads)
	// 1. LLMs query
	// 2. llm_filters junction table
	// 3. filters table
	// 4. llm_plugins junction table
	// 5. plugins table
	assert.LessOrEqual(t, queryLogger.QueryCount, 5,
		"Query count should be minimal with proper preloading (got %d queries)", queryLogger.QueryCount)
}

func TestLLMService_GetActiveLLMsInNamespace_PreventN1Queries(t *testing.T) {
	db, queryLogger := setupN1TestDB(t)
	createTestDataForN1(t, db)

	service := NewService(db)

	// Reset query counter
	queryLogger.QueryCount = 0

	// Call GetActiveLLMsInNamespace which should preload plugins and filters
	llms, err := service.GetActiveLLMsInNamespace("")
	require.NoError(t, err)
	assert.Len(t, llms, 3)

	// Verify that plugins are preloaded
	for _, llm := range llms {
		assert.Len(t, llm.Plugins, 2, "Each LLM should have 2 plugins preloaded")
		assert.Len(t, llm.Filters, 2, "Each LLM should have 2 filters preloaded")
	}

	// Should be ≤5 queries with proper preloading (LLMs + 2 many2many preloads)
	// 1. LLMs query
	// 2. llm_filters junction table
	// 3. filters table
	// 4. llm_plugins junction table
	// 5. plugins table
	assert.LessOrEqual(t, queryLogger.QueryCount, 5,
		"Query count should be minimal with proper preloading (got %d queries)", queryLogger.QueryCount)
}

func TestPluginService_ListPlugins_PreventN1Queries(t *testing.T) {
	db, queryLogger := setupN1TestDB(t)
	createTestDataForN1(t, db)

	// Create plugin service
	pluginService := NewPluginService(db)

	// Reset query counter
	queryLogger.QueryCount = 0

	// Call ListPlugins which should preload LLMs
	plugins, totalCount, err := pluginService.ListPlugins(1, 10, "", true, "")
	require.NoError(t, err)
	assert.Equal(t, int64(5), totalCount)
	assert.Len(t, plugins, 5)

	// Verify that LLMs are preloaded
	for _, plugin := range plugins {
		// Each plugin should have some LLMs associated
		if len(plugin.LLMs) > 0 {
			for _, llm := range plugin.LLMs {
				assert.NotEmpty(t, llm.Name)
			}
		}
	}

	// With proper preloading, we should have a minimal number of queries:
	// 1. COUNT query for pagination
	// 2. Main Plugin query with preloads
	assert.LessOrEqual(t, queryLogger.QueryCount, 4,
		"Query count should be minimal with proper preloading (got %d queries)", queryLogger.QueryCount)
}