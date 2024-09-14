package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForDataCatalogues(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestDataCatalogueService(t *testing.T) {
	db := setupTestDBForDataCatalogues(t)
	service := NewService(db)

	// Test CreateDataCatalogue
	dataCatalogue, err := service.CreateDataCatalogue("Test Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)
	assert.NotNil(t, dataCatalogue)
	assert.NotZero(t, dataCatalogue.ID)
	assert.Equal(t, "Test Catalogue", dataCatalogue.Name)

	// Test GetDataCatalogueByID
	fetchedDataCatalogue, err := service.GetDataCatalogueByID(dataCatalogue.ID)
	assert.NoError(t, err)
	assert.Equal(t, dataCatalogue.ID, fetchedDataCatalogue.ID)
	assert.Equal(t, dataCatalogue.Name, fetchedDataCatalogue.Name)

	// Test UpdateDataCatalogue
	updatedDataCatalogue, err := service.UpdateDataCatalogue(dataCatalogue.ID, "Updated Catalogue", "Updated Short", "Updated Long", "updated-icon.png")
	assert.NoError(t, err)
	assert.Equal(t, dataCatalogue.ID, updatedDataCatalogue.ID)
	assert.Equal(t, "Updated Catalogue", updatedDataCatalogue.Name)

	// Test AddTagToDataCatalogue
	tag, err := service.CreateTag("Test Tag")
	assert.NoError(t, err)
	err = service.AddTagToDataCatalogue(dataCatalogue.ID, tag.ID)
	assert.NoError(t, err)

	// Verify tag was added
	fetchedDataCatalogue, err = service.GetDataCatalogueByID(dataCatalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDataCatalogue.Tags, 1)
	assert.Equal(t, tag.ID, fetchedDataCatalogue.Tags[0].ID)

	// Test RemoveTagFromDataCatalogue
	err = service.RemoveTagFromDataCatalogue(dataCatalogue.ID, tag.ID)
	assert.NoError(t, err)

	// Verify tag was removed
	fetchedDataCatalogue, err = service.GetDataCatalogueByID(dataCatalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDataCatalogue.Tags, 0)

	// Test AddDatasourceToDataCatalogue
	datasource, err := service.CreateDatasource(
		"Test Datasource", "Short Desc",
		"Long Desc", "icon.png", "https://example.com", 75, 1, []string{},
		"conn_string", "source_type", "api_key", "dbname",
		"embed_vendor", "embed_url", "embed_api_key", "embed_model", true)
	assert.NoError(t, err)
	err = service.AddDatasourceToDataCatalogue(dataCatalogue.ID, datasource.ID)
	assert.NoError(t, err)

	// Verify datasource was added
	fetchedDataCatalogue, err = service.GetDataCatalogueByID(dataCatalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDataCatalogue.Datasources, 1)
	assert.Equal(t, datasource.ID, fetchedDataCatalogue.Datasources[0].ID)

	// Test RemoveDatasourceFromDataCatalogue
	err = service.RemoveDatasourceFromDataCatalogue(dataCatalogue.ID, datasource.ID)
	assert.NoError(t, err)

	// Verify datasource was removed
	fetchedDataCatalogue, err = service.GetDataCatalogueByID(dataCatalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedDataCatalogue.Datasources, 0)

	// Test GetAllDataCatalogues
	allDataCatalogues, err := service.GetAllDataCatalogues()
	assert.NoError(t, err)
	assert.Len(t, allDataCatalogues, 1)
	assert.Equal(t, dataCatalogue.ID, allDataCatalogues[0].ID)

	// Test SearchDataCatalogues
	searchedDataCatalogues, err := service.SearchDataCatalogues("Updated")
	assert.NoError(t, err)
	assert.Len(t, searchedDataCatalogues, 1)
	assert.Equal(t, dataCatalogue.ID, searchedDataCatalogues[0].ID)

	// Test GetDataCataloguesByTag
	err = service.AddTagToDataCatalogue(dataCatalogue.ID, tag.ID)
	assert.NoError(t, err)
	dataCataloguesByTag, err := service.GetDataCataloguesByTag("Test Tag")
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesByTag, 1)
	assert.Equal(t, dataCatalogue.ID, dataCataloguesByTag[0].ID)

	// Test GetDataCataloguesByDatasource
	err = service.AddDatasourceToDataCatalogue(dataCatalogue.ID, datasource.ID)
	assert.NoError(t, err)
	dataCataloguesByDatasource, err := service.GetDataCataloguesByDatasource(datasource.ID)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesByDatasource, 1)
	assert.Equal(t, dataCatalogue.ID, dataCataloguesByDatasource[0].ID)

	// Test DeleteDataCatalogue
	err = service.DeleteDataCatalogue(dataCatalogue.ID)
	assert.NoError(t, err)

	// Verify data catalogue is deleted
	_, err = service.GetDataCatalogueByID(dataCatalogue.ID)
	assert.Error(t, err)
}

func TestDataCatalogueService_MultipleDataCataloguesScenario(t *testing.T) {
	db := setupTestDBForDataCatalogues(t)
	service := NewService(db)

	// Create multiple data catalogues
	dc1, _ := service.CreateDataCatalogue("Catalogue 1", "Short 1", "Long 1", "icon1.png")
	dc2, _ := service.CreateDataCatalogue("Catalogue 2", "Short 2", "Long 2", "icon2.png")
	dc3, _ := service.CreateDataCatalogue("Catalogue 3", "Short 3", "Long 3", "icon3.png")

	// Test GetAllDataCatalogues
	allDataCatalogues, err := service.GetAllDataCatalogues()
	assert.NoError(t, err)
	assert.Len(t, allDataCatalogues, 3)

	// Test SearchDataCatalogues
	searchedDataCatalogues, err := service.SearchDataCatalogues("Catalogue")
	assert.NoError(t, err)
	assert.Len(t, searchedDataCatalogues, 3)

	// Create tags and add to data catalogues
	tag1, _ := service.CreateTag("Tag 1")
	tag2, _ := service.CreateTag("Tag 2")

	err = service.AddTagToDataCatalogue(dc1.ID, tag1.ID)
	assert.NoError(t, err)
	err = service.AddTagToDataCatalogue(dc2.ID, tag1.ID)
	assert.NoError(t, err)
	err = service.AddTagToDataCatalogue(dc2.ID, tag2.ID)
	assert.NoError(t, err)
	err = service.AddTagToDataCatalogue(dc3.ID, tag2.ID)
	assert.NoError(t, err)

	// Test GetDataCataloguesByTag
	dataCataloguesTag1, err := service.GetDataCataloguesByTag("Tag 1")
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesTag1, 2)
	assert.ElementsMatch(t, []uint{dc1.ID, dc2.ID}, []uint{dataCataloguesTag1[0].ID, dataCataloguesTag1[1].ID})

	dataCataloguesTag2, err := service.GetDataCataloguesByTag("Tag 2")
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesTag2, 2)
	assert.ElementsMatch(t, []uint{dc2.ID, dc3.ID}, []uint{dataCataloguesTag2[0].ID, dataCataloguesTag2[1].ID})

	// Create datasources and add to data catalogues
	ds1, _ := service.CreateDatasource("Datasource 1", "Short 1", "Long 1", "icon1.png", "https://example1.com", 75, 1, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	ds2, _ := service.CreateDatasource("Datasource 2", "Short 2", "Long 2", "icon2.png", "https://example2.com", 80, 1, []string{}, "conn_string2", "source_type2", "api_key2", "db2", "embed_vendor2", "embed_url2", "embed_api_key2", "embed_model2", true)

	err = service.AddDatasourceToDataCatalogue(dc1.ID, ds1.ID)
	assert.NoError(t, err)
	err = service.AddDatasourceToDataCatalogue(dc2.ID, ds1.ID)
	assert.NoError(t, err)
	err = service.AddDatasourceToDataCatalogue(dc2.ID, ds2.ID)
	assert.NoError(t, err)

	// Test GetDataCataloguesByDatasource
	dataCataloguesDs1, err := service.GetDataCataloguesByDatasource(ds1.ID)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesDs1, 2)
	assert.ElementsMatch(t, []uint{dc1.ID, dc2.ID}, []uint{dataCataloguesDs1[0].ID, dataCataloguesDs1[1].ID})

	dataCataloguesDs2, err := service.GetDataCataloguesByDatasource(ds2.ID)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesDs2, 1)
	assert.Equal(t, dc2.ID, dataCataloguesDs2[0].ID)
}
