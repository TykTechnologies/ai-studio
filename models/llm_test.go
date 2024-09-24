package models

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLLM_NewLLM(t *testing.T) {
	llm := NewLLM()
	assert.NotNil(t, llm)
}

func TestLLM_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	llm := &LLM{
		Name:        "TestLLM",
		APIKey:      "test-api-key",
		APIEndpoint: "https://api.test.com",
	}
	err := llm.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, llm.ID)

	// Get
	fetchedLLM := NewLLM()
	err = fetchedLLM.Get(db, llm.ID)
	assert.NoError(t, err)
	assert.Equal(t, llm.Name, fetchedLLM.Name)
	assert.Equal(t, llm.APIKey, fetchedLLM.APIKey)
	assert.Equal(t, llm.APIEndpoint, fetchedLLM.APIEndpoint)

	// Update
	llm.Name = "UpdatedTestLLM"
	err = llm.Update(db)
	assert.NoError(t, err)

	err = fetchedLLM.Get(db, llm.ID)
	assert.NoError(t, err)
	assert.Equal(t, "UpdatedTestLLM", fetchedLLM.Name)

	// Delete
	err = llm.Delete(db)
	assert.NoError(t, err)

	err = fetchedLLM.Get(db, llm.ID)
	assert.Error(t, err) // Should return an error as the LLM is deleted
}

func TestLLM_GetByName(t *testing.T) {
	db := setupTestDB(t)

	llm := &LLM{
		Name:        "UniqueLLM",
		APIKey:      "unique-api-key",
		APIEndpoint: "https://api.unique.com",
	}
	err := llm.Create(db)
	assert.NoError(t, err)

	fetchedLLM := NewLLM()
	err = fetchedLLM.GetByName(db, "UniqueLLM")
	assert.NoError(t, err)
	assert.Equal(t, llm.ID, fetchedLLM.ID)
	assert.Equal(t, llm.Name, fetchedLLM.Name)
	assert.Equal(t, llm.APIKey, fetchedLLM.APIKey)
	assert.Equal(t, llm.APIEndpoint, fetchedLLM.APIEndpoint)

	// Test with non-existent name
	err = fetchedLLM.GetByName(db, "NonExistentLLM")
	assert.Error(t, err)
}

func TestLLMs_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMs
	llms := []LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com"},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com"},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com"},
	}
	for _, l := range llms {
		err := db.Create(&l).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedLLMs LLMs
	_, _, err := fetchedLLMs.GetAll(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, fetchedLLMs, 3)
	assert.Equal(t, "LLM1", fetchedLLMs[0].Name)
	assert.Equal(t, "LLM2", fetchedLLMs[1].Name)
	assert.Equal(t, "LLM3", fetchedLLMs[2].Name)
}

func TestLLMs_GetByNameStub(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMs
	llms := []LLM{
		{Name: "GPT-3", APIKey: "key1", APIEndpoint: "https://api1.com"},
		{Name: "GPT-4", APIKey: "key2", APIEndpoint: "https://api2.com"},
		{Name: "BERT", APIKey: "key3", APIEndpoint: "https://api3.com"},
	}
	for _, l := range llms {
		err := db.Create(&l).Error
		assert.NoError(t, err)
	}

	// Test GetByNameStub
	var fetchedLLMs LLMs
	err := fetchedLLMs.GetByNameStub(db, "GPT")
	assert.NoError(t, err)
	assert.Len(t, fetchedLLMs, 2)
	assert.Equal(t, "GPT-3", fetchedLLMs[0].Name)
	assert.Equal(t, "GPT-4", fetchedLLMs[1].Name)

	// Test with a different stub
	fetchedLLMs = LLMs{}
	err = fetchedLLMs.GetByNameStub(db, "BERT")
	assert.NoError(t, err)
	assert.Len(t, fetchedLLMs, 1)
	assert.Equal(t, "BERT", fetchedLLMs[0].Name)

	// Test with a stub that doesn't match any LLMs
	fetchedLLMs = LLMs{}
	err = fetchedLLMs.GetByNameStub(db, "XYZ")
	assert.NoError(t, err)
	assert.Len(t, fetchedLLMs, 0)
}

func TestLLMs_GetByMaxPrivacyScore(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMs with different privacy scores
	llms := []LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", PrivacyScore: 50},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", PrivacyScore: 75},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", PrivacyScore: 90},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", PrivacyScore: 30},
	}
	for _, l := range llms {
		err := db.Create(&l).Error
		assert.NoError(t, err)
	}

	// Test GetByMaxPrivacyScore with different scores
	testCases := []struct {
		maxScore      int
		expectedCount int
		expectedNames []string
	}{
		{100, 4, []string{"LLM1", "LLM2", "LLM3", "LLM4"}},
		{80, 3, []string{"LLM1", "LLM2", "LLM4"}},
		{60, 2, []string{"LLM1", "LLM4"}},
		{40, 1, []string{"LLM4"}},
		{20, 0, []string{}},
	}

	for _, tc := range testCases {
		var fetchedLLMs LLMs
		err := fetchedLLMs.GetByMaxPrivacyScore(db, tc.maxScore)
		assert.NoError(t, err)
		assert.Len(t, fetchedLLMs, tc.expectedCount)

		// Check if the returned LLMs match the expected names
		var fetchedNames []string
		for _, llm := range fetchedLLMs {
			fetchedNames = append(fetchedNames, llm.Name)
		}
		assert.ElementsMatch(t, tc.expectedNames, fetchedNames)

		// Verify that all returned LLMs have a privacy score less than or equal to the max score
		for _, llm := range fetchedLLMs {
			assert.LessOrEqual(t, llm.PrivacyScore, tc.maxScore)
		}
	}
}

func TestLLMs_GetByMinPrivacyScore(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMs with different privacy scores
	llms := []LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", PrivacyScore: 50},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", PrivacyScore: 75},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", PrivacyScore: 90},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", PrivacyScore: 30},
	}
	for _, l := range llms {
		err := db.Create(&l).Error
		assert.NoError(t, err)
	}

	// Test GetByMinPrivacyScore with different scores
	testCases := []struct {
		minScore      int
		expectedCount int
		expectedNames []string
	}{
		{0, 4, []string{"LLM1", "LLM2", "LLM3", "LLM4"}},
		{40, 3, []string{"LLM1", "LLM2", "LLM3"}},
		{70, 2, []string{"LLM2", "LLM3"}},
		{80, 1, []string{"LLM3"}},
		{95, 0, []string{}},
	}

	for _, tc := range testCases {
		var fetchedLLMs LLMs
		err := fetchedLLMs.GetByMinPrivacyScore(db, tc.minScore)
		assert.NoError(t, err)
		assert.Len(t, fetchedLLMs, tc.expectedCount)

		// Check if the returned LLMs match the expected names
		var fetchedNames []string
		for _, llm := range fetchedLLMs {
			fetchedNames = append(fetchedNames, llm.Name)
		}
		assert.ElementsMatch(t, tc.expectedNames, fetchedNames)

		// Verify that all returned LLMs have a privacy score greater than or equal to the min score
		for _, llm := range fetchedLLMs {
			assert.GreaterOrEqual(t, llm.PrivacyScore, tc.minScore)
		}
	}
}

func TestLLMs_GetByPrivacyScoreRange(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMs with different privacy scores
	llms := []LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", PrivacyScore: 50},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", PrivacyScore: 75},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", PrivacyScore: 90},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", PrivacyScore: 30},
		{Name: "LLM5", APIKey: "key5", APIEndpoint: "https://api5.com", PrivacyScore: 60},
	}
	for _, l := range llms {
		err := db.Create(&l).Error
		assert.NoError(t, err)
	}

	// Test GetByPrivacyScoreRange with different score ranges
	testCases := []struct {
		minScore      int
		maxScore      int
		expectedCount int
		expectedNames []string
	}{
		{0, 100, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{40, 80, 3, []string{"LLM1", "LLM2", "LLM5"}},
		{70, 90, 2, []string{"LLM2", "LLM3"}},
		{30, 50, 2, []string{"LLM1", "LLM4"}},
		{95, 100, 0, []string{}},
		{0, 25, 0, []string{}},
	}

	for _, tc := range testCases {
		var fetchedLLMs LLMs
		err := fetchedLLMs.GetByPrivacyScoreRange(db, tc.minScore, tc.maxScore)
		assert.NoError(t, err)
		assert.Len(t, fetchedLLMs, tc.expectedCount)

		// Check if the returned LLMs match the expected names
		var fetchedNames []string
		for _, llm := range fetchedLLMs {
			fetchedNames = append(fetchedNames, llm.Name)
		}
		assert.ElementsMatch(t, tc.expectedNames, fetchedNames)

		// Verify that all returned LLMs have a privacy score within the specified range
		for _, llm := range fetchedLLMs {
			assert.GreaterOrEqual(t, llm.PrivacyScore, tc.minScore)
			assert.LessOrEqual(t, llm.PrivacyScore, tc.maxScore)
		}
	}

	// Test with invalid range (min > max)
	var fetchedLLMs LLMs
	err := fetchedLLMs.GetByPrivacyScoreRange(db, 80, 70)
	assert.NoError(t, err)
	assert.Len(t, fetchedLLMs, 0)
}

func TestLLMs_GetAll_Pagination(t *testing.T) {
	db := setupTestDB(t)

	// Create 25 test LLMs
	for i := 1; i <= 25; i++ {
		llm := &LLM{
			Name:        fmt.Sprintf("LLM%d", i),
			APIKey:      fmt.Sprintf("key%d", i),
			APIEndpoint: fmt.Sprintf("https://api%d.com", i),
		}
		err := llm.Create(db)
		assert.NoError(t, err)
	}

	testCases := []struct {
		name           string
		pageSize       int
		pageNumber     int
		all            bool
		expectedCount  int
		expectedTotal  int64
		expectedPages  int
		expectedFirst  string
		expectedLast   string
	}{
		{
			name:           "First page of 10",
			pageSize:       10,
			pageNumber:     1,
			all:            false,
			expectedCount:  10,
			expectedTotal:  25,
			expectedPages:  3,
			expectedFirst:  "LLM1",
			expectedLast:   "LLM10",
		},
		{
			name:           "Second page of 10",
			pageSize:       10,
			pageNumber:     2,
			all:            false,
			expectedCount:  10,
			expectedTotal:  25,
			expectedPages:  3,
			expectedFirst:  "LLM11",
			expectedLast:   "LLM20",
		},
		{
			name:           "Last page of 10",
			pageSize:       10,
			pageNumber:     3,
			all:            false,
			expectedCount:  5,
			expectedTotal:  25,
			expectedPages:  3,
			expectedFirst:  "LLM21",
			expectedLast:   "LLM25",
		},
		{
			name:           "Page size larger than total",
			pageSize:       30,
			pageNumber:     1,
			all:            false,
			expectedCount:  25,
			expectedTotal:  25,
			expectedPages:  1,
			expectedFirst:  "LLM1",
			expectedLast:   "LLM25",
		},
		{
			name:           "Get all LLMs",
			pageSize:       10,
			pageNumber:     1,
			all:            true,
			expectedCount:  25,
			expectedTotal:  25,
			expectedPages:  3,
			expectedFirst:  "LLM1",
			expectedLast:   "LLM25",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fetchedLLMs LLMs
			totalCount, totalPages, err := fetchedLLMs.GetAll(db, tc.pageSize, tc.pageNumber, tc.all)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTotal, totalCount)
			assert.Equal(t, tc.expectedPages, totalPages)
			assert.Len(t, fetchedLLMs, tc.expectedCount)

			if len(fetchedLLMs) > 0 {
				assert.Equal(t, tc.expectedFirst, fetchedLLMs[0].Name)
				assert.Equal(t, tc.expectedLast, fetchedLLMs[len(fetchedLLMs)-1].Name)
			}
		})
	}
}
