package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCatalogue_NewCatalogue(t *testing.T) {
	catalogue := NewCatalogue()
	assert.NotNil(t, catalogue)
}

func TestCatalogue_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	catalogue := &Catalogue{Name: "Test Catalogue"}
	err := catalogue.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, catalogue.ID)

	// Get
	fetchedCatalogue := NewCatalogue()
	err = fetchedCatalogue.Get(db, catalogue.ID)
	assert.NoError(t, err)
	assert.Equal(t, catalogue.Name, fetchedCatalogue.Name)

	// Update
	catalogue.Name = "Updated Test Catalogue"
	err = catalogue.Update(db)
	assert.NoError(t, err)

	err = fetchedCatalogue.Get(db, catalogue.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Test Catalogue", fetchedCatalogue.Name)

	// Delete
	err = catalogue.Delete(db)
	assert.NoError(t, err)

	err = fetchedCatalogue.Get(db, catalogue.ID)
	assert.Error(t, err) // Should return an error as the catalogue is deleted
}

func TestCatalogue_LLMAssociation(t *testing.T) {
	db := setupTestDB(t)

	catalogue := &Catalogue{Name: "Test Catalogue"}
	err := catalogue.Create(db)
	assert.NoError(t, err)

	llm := &LLM{Name: "Test LLM", APIKey: "test-key", APIEndpoint: "https://test.com"}
	err = llm.Create(db)
	assert.NoError(t, err)

	// Add LLM
	err = catalogue.AddLLM(db, llm)
	assert.NoError(t, err)

	// Get LLMs
	err = catalogue.GetCatalogueLLMs(db)
	assert.NoError(t, err)
	assert.Len(t, catalogue.LLMs, 1)
	assert.Equal(t, llm.ID, catalogue.LLMs[0].ID)

	// Remove LLM
	err = catalogue.RemoveLLM(db, llm)
	assert.NoError(t, err)

	err = catalogue.GetCatalogueLLMs(db)
	assert.NoError(t, err)
	assert.Len(t, catalogue.LLMs, 0)
}

func TestCatalogues_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test catalogues
	catalogues := []Catalogue{
		{Name: "Catalogue 1"},
		{Name: "Catalogue 2"},
		{Name: "Catalogue 3"},
	}
	for _, c := range catalogues {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedCatalogues Catalogues
	_, _, err := fetchedCatalogues.GetAll(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, fetchedCatalogues, 3)
	assert.Equal(t, "Catalogue 1", fetchedCatalogues[0].Name)
	assert.Equal(t, "Catalogue 2", fetchedCatalogues[1].Name)
	assert.Equal(t, "Catalogue 3", fetchedCatalogues[2].Name)
}

func TestCatalogues_GetByNameStub(t *testing.T) {
	db := setupTestDB(t)

	// Create some test catalogues
	catalogues := []Catalogue{
		{Name: "AI Models"},
		{Name: "Machine Learning"},
		{Name: "AI Assistants"},
		{Name: "Natural Language Processing"},
	}
	for _, c := range catalogues {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Test GetByNameStub
	var fetchedCatalogues Catalogues
	err := fetchedCatalogues.GetByNameStub(db, "AI")
	assert.NoError(t, err)
	assert.Len(t, fetchedCatalogues, 2)
	assert.Equal(t, "AI Models", fetchedCatalogues[0].Name)
	assert.Equal(t, "AI Assistants", fetchedCatalogues[1].Name)

	// Test with a different stub
	fetchedCatalogues = Catalogues{}
	err = fetchedCatalogues.GetByNameStub(db, "Machine")
	assert.NoError(t, err)
	assert.Len(t, fetchedCatalogues, 1)
	assert.Equal(t, "Machine Learning", fetchedCatalogues[0].Name)

	// Test with a stub that doesn't match any catalogues
	fetchedCatalogues = Catalogues{}
	err = fetchedCatalogues.GetByNameStub(db, "Quantum")
	assert.NoError(t, err)
	assert.Len(t, fetchedCatalogues, 0)
}
