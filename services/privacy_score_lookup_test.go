package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// countQueries counts SELECT statements issued while fn runs.
func countQueries(t *testing.T, db *gorm.DB, fn func()) int {
	t.Helper()

	count := 0
	const cb = "test:count_queries"
	err := db.Callback().Query().After("gorm:query").Register(cb, func(tx *gorm.DB) {
		count++
	})
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, db.Callback().Query().Remove(cb))
	}()

	fn()
	return count
}

// The check used to fetch every LLM, datasource and tool one row at a time, so
// its cost grew with the number of resources on the app -- on every create and
// every update. This pins the bulk form: at most one query per resource kind,
// regardless of how many ids are passed.
func TestValidatePrivacyScores_QueryCount(t *testing.T) {
	service, _ := setupAppTest(t)

	var llmIDs, dsIDs, toolIDs []uint
	for i := 0; i < 5; i++ {
		llm := createTestAppLLM(t, service, "qc-llm-"+string(rune('a'+i)), 50)
		ds := createTestAppDatasource(t, service, "qc-ds-"+string(rune('a'+i)), 10)
		tool := createTestAppToolWithPrivacy(t, service, "qc-tool-"+string(rune('a'+i)), 10)
		llmIDs = append(llmIDs, llm.ID)
		dsIDs = append(dsIDs, ds.ID)
		toolIDs = append(toolIDs, tool.ID)
	}

	var err error
	queries := countQueries(t, service.DB, func() {
		err = service.validatePrivacyScores(dsIDs, llmIDs, toolIDs)
	})

	assert.NoError(t, err)
	assert.LessOrEqual(t, queries, 3,
		"one query per resource kind at most; 15 resources must not mean 15 queries")
}

func TestValidatePrivacyScores_SkipsToolQueryWithoutProviders(t *testing.T) {
	service, _ := setupAppTest(t)

	tool := createTestAppToolWithPrivacy(t, service, "qc-skip-tool", 80)

	var err error
	queries := countQueries(t, service.DB, func() {
		err = service.validatePrivacyScores(nil, nil, []uint{tool.ID})
	})

	assert.NoError(t, err)
	assert.Equal(t, 0, queries,
		"with no providers to compare against, tools are not fetched at all")
}

func TestLoadPrivacyScores_MissingIDIsAnError(t *testing.T) {
	service, _ := setupAppTest(t)
	llm := createTestAppLLM(t, service, "qc-present", 10)

	// Find() returns fewer rows rather than failing, so a bulk load has to
	// check the count or a missing resource would silently score zero.
	t.Run("llm", func(t *testing.T) {
		_, err := service.loadLLMPrivacyScores([]uint{llm.ID, 999999})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "999999")
	})

	t.Run("datasource", func(t *testing.T) {
		_, err := service.loadDatasourcePrivacyScores([]uint{999999})
		assert.Error(t, err)
	})

	t.Run("tool", func(t *testing.T) {
		_, err := service.loadToolPrivacyScores([]uint{999999})
		assert.Error(t, err)
	})

	t.Run("empty id list issues no query and no error", func(t *testing.T) {
		queries := countQueries(t, service.DB, func() {
			_, err := service.loadLLMPrivacyScores(nil)
			assert.NoError(t, err)
		})
		assert.Equal(t, 0, queries)
	})
}
