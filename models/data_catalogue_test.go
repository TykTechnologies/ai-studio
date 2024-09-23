package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataCatalogue_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	dc := &DataCatalogue{
		Name:             "Test Catalogue",
		ShortDescription: "Short desc",
		LongDescription:  "Long desc",
		Icon:             "icon.png",
	}
	err := dc.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, dc.ID)

	// Read
	fetchedDC := NewDataCatalogue()
	err = fetchedDC.Get(db, dc.ID)
	assert.NoError(t, err)
	assert.Equal(t, dc.Name, fetchedDC.Name)

	// Update
	dc.Name = "Updated Catalogue"
	err = dc.Update(db)
	assert.NoError(t, err)
	err = fetchedDC.Get(db, dc.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Catalogue", fetchedDC.Name)

	// Delete
	err = dc.Delete(db)
	assert.NoError(t, err)
	err = fetchedDC.Get(db, dc.ID)
	assert.Error(t, err) // Should return an error as the catalogue is deleted
}

func TestDataCatalogue_TagAssociation(t *testing.T) {
	db := setupTestDB(t)

	dc := &DataCatalogue{Name: "Test Catalogue"}
	err := dc.Create(db)
	assert.NoError(t, err)

	tag := &Tag{Name: "Test Tag"}
	err = tag.Create(db)
	assert.NoError(t, err)

	// Add Tag
	err = dc.AddTag(db, tag)
	assert.NoError(t, err)

	// Verify Tag was added
	fetchedDC := NewDataCatalogue()
	err = fetchedDC.Get(db, dc.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDC.Tags, 1)
	assert.Equal(t, tag.ID, fetchedDC.Tags[0].ID)

	// Remove Tag
	err = dc.RemoveTag(db, tag)
	assert.NoError(t, err)

	// Verify Tag was removed
	err = fetchedDC.Get(db, dc.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDC.Tags, 0)
}

func TestDataCatalogue_DatasourceAssociation(t *testing.T) {
	db := setupTestDB(t)

	dc := &DataCatalogue{Name: "Test Catalogue"}
	err := dc.Create(db)
	assert.NoError(t, err)

	ds := &Datasource{Name: "Test Datasource", Active: true}
	err = ds.Create(db)
	assert.NoError(t, err)

	// Add Datasource
	err = dc.AddDatasource(db, ds)
	assert.NoError(t, err)

	// Verify Datasource was added
	fetchedDC := NewDataCatalogue()
	err = fetchedDC.Get(db, dc.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDC.Datasources, 1)
	assert.Equal(t, ds.ID, fetchedDC.Datasources[0].ID)

	// Remove Datasource
	err = dc.RemoveDatasource(db, ds)
	assert.NoError(t, err)

	// Verify Datasource was removed
	err = fetchedDC.Get(db, dc.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDC.Datasources, 0)
}

func TestDataCatalogues_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test data catalogues
	catalogues := []DataCatalogue{
		{Name: "Catalogue 1"},
		{Name: "Catalogue 2"},
		{Name: "Catalogue 3"},
	}
	for _, c := range catalogues {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedCatalogues DataCatalogues
	_, _, err := fetchedCatalogues.GetAll(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, fetchedCatalogues, 3)
	assert.Equal(t, "Catalogue 1", fetchedCatalogues[0].Name)
	assert.Equal(t, "Catalogue 2", fetchedCatalogues[1].Name)
	assert.Equal(t, "Catalogue 3", fetchedCatalogues[2].Name)
}

func TestDataCatalogues_Search(t *testing.T) {
	db := setupTestDB(t)

	// Create some test data catalogues
	catalogues := []DataCatalogue{
		{Name: "Apple Catalogue", ShortDescription: "Fruit catalogue", LongDescription: "A catalogue of apple varieties"},
		{Name: "Banana Database", ShortDescription: "Yellow fruit data", LongDescription: "Database of banana types"},
		{Name: "Cherry Collection", ShortDescription: "Red info", LongDescription: "Collection of cherry information"},
	}
	for _, c := range catalogues {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Test Search
	var results DataCatalogues
	err := results.Search(db, "apple")
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Apple Catalogue", results[0].Name)

	results = DataCatalogues{}
	err = results.Search(db, "fruit")
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	results = DataCatalogues{}
	err = results.Search(db, "database")
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Banana Database", results[0].Name)
}

func TestDataCatalogues_GetByTag(t *testing.T) {
	db := setupTestDB(t)

	// Create test data catalogues and tags
	dc1 := &DataCatalogue{Name: "Catalogue 1"}
	dc2 := &DataCatalogue{Name: "Catalogue 2"}
	err := dc1.Create(db)
	assert.NoError(t, err)
	err = dc2.Create(db)
	assert.NoError(t, err)

	tag1 := &Tag{Name: "Tag1"}
	tag2 := &Tag{Name: "Tag2"}
	err = tag1.Create(db)
	assert.NoError(t, err)
	err = tag2.Create(db)
	assert.NoError(t, err)

	err = dc1.AddTag(db, tag1)
	assert.NoError(t, err)
	err = dc2.AddTag(db, tag2)
	assert.NoError(t, err)

	// Test GetByTag
	var results DataCatalogues
	err = results.GetByTag(db, "Tag1")
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Catalogue 1", results[0].Name)

	results = DataCatalogues{}
	err = results.GetByTag(db, "Tag2")
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Catalogue 2", results[0].Name)
}

func TestDataCatalogues_GetByDatasource(t *testing.T) {
	db := setupTestDB(t)

	// Create test data catalogues and datasources
	dc1 := &DataCatalogue{Name: "Catalogue 1"}
	dc2 := &DataCatalogue{Name: "Catalogue 2"}
	err := dc1.Create(db)
	assert.NoError(t, err)
	err = dc2.Create(db)
	assert.NoError(t, err)

	ds1 := &Datasource{Name: "Datasource 1"}
	ds2 := &Datasource{Name: "Datasource 2"}
	err = ds1.Create(db)
	assert.NoError(t, err)
	err = ds2.Create(db)
	assert.NoError(t, err)

	err = dc1.AddDatasource(db, ds1)
	assert.NoError(t, err)
	err = dc2.AddDatasource(db, ds2)
	assert.NoError(t, err)

	// Test GetByDatasource
	var results DataCatalogues
	err = results.GetByDatasource(db, ds1.ID)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Catalogue 1", results[0].Name)

	results = DataCatalogues{}
	err = results.GetByDatasource(db, ds2.ID)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Catalogue 2", results[0].Name)
}
