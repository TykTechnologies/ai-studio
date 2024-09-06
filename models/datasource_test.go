package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatasource_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Test Create
	ds := &Datasource{
		Name:             "Test Datasource",
		ShortDescription: "Short desc",
		LongDescription:  "Long desc",
		Icon:             "icon.png",
		Url:              "https://example.com",
		PrivacyScore:     75,
		UserID:           1,
	}
	err := ds.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, ds.ID)

	// Test Get
	fetchedDS := NewDatasource()
	err = fetchedDS.Get(db, ds.ID)
	assert.NoError(t, err)
	assert.Equal(t, ds.Name, fetchedDS.Name)

	// Test Update
	ds.Name = "Updated Datasource"
	err = ds.Update(db)
	assert.NoError(t, err)
	err = fetchedDS.Get(db, ds.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Datasource", fetchedDS.Name)

	// Test Delete
	err = ds.Delete(db)
	assert.NoError(t, err)
	err = fetchedDS.Get(db, ds.ID)
	assert.Error(t, err)
}

func TestDatasources_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create test datasources
	datasources := []Datasource{
		{Name: "DS1", UserID: 1},
		{Name: "DS2", UserID: 1},
		{Name: "DS3", UserID: 2},
	}
	for _, ds := range datasources {
		err := db.Create(&ds).Error
		assert.NoError(t, err)
	}

	var fetchedDS Datasources
	err := fetchedDS.GetAll(db)
	assert.NoError(t, err)
	assert.Len(t, fetchedDS, 3)
}

func TestDatasources_Search(t *testing.T) {
	db := setupTestDB(t)

	datasources := []Datasource{
		{Name: "Apple DS", ShortDescription: "Fruit", LongDescription: "A tasty fruit"},
		{Name: "Banana DS", ShortDescription: "Yellow fruit", LongDescription: "Long yellow fruit"},
		{Name: "Cherry DS", ShortDescription: "Red fruit", LongDescription: "Small red fruit"},
	}
	for _, ds := range datasources {
		err := db.Create(&ds).Error
		assert.NoError(t, err)
	}

	var searchResults Datasources
	err := searchResults.Search(db, "yellow")
	assert.NoError(t, err)
	assert.Len(t, searchResults, 1)
	assert.Equal(t, "Banana DS", searchResults[0].Name)
}

func TestDatasources_GetByTag(t *testing.T) {
	db := setupTestDB(t)

	// Create datasources with tags
	ds1 := &Datasource{Name: "DS1"}
	ds2 := &Datasource{Name: "DS2"}
	err := db.Create(ds1).Error
	assert.NoError(t, err)
	err = db.Create(ds2).Error
	assert.NoError(t, err)

	err = ds1.AddTags(db, []string{"tag1", "tag2"})
	assert.NoError(t, err)
	err = ds2.AddTags(db, []string{"tag2", "tag3"})
	assert.NoError(t, err)

	var taggedDS Datasources
	err = taggedDS.GetByTag(db, "tag2")
	assert.NoError(t, err)
	assert.Len(t, taggedDS, 2)
}

func TestDatasource_AddTags(t *testing.T) {
	db := setupTestDB(t)

	ds := &Datasource{Name: "Test DS"}
	err := db.Create(ds).Error
	assert.NoError(t, err)

	err = ds.AddTags(db, []string{"new_tag1", "new_tag2"})
	assert.NoError(t, err)

	var fetchedDS Datasource
	err = db.Preload("Tags").First(&fetchedDS, ds.ID).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedDS.Tags, 2)
}

func TestDatasources_GetByPrivacyScore(t *testing.T) {
	db := setupTestDB(t)

	datasources := []Datasource{
		{Name: "DS1", PrivacyScore: 30},
		{Name: "DS2", PrivacyScore: 50},
		{Name: "DS3", PrivacyScore: 70},
		{Name: "DS4", PrivacyScore: 90},
	}
	for _, ds := range datasources {
		err := db.Create(&ds).Error
		assert.NoError(t, err)
	}

	var minScoreDS Datasources
	err := minScoreDS.GetByMinPrivacyScore(db, 60)
	assert.NoError(t, err)
	assert.Len(t, minScoreDS, 2)

	var maxScoreDS Datasources
	err = maxScoreDS.GetByMaxPrivacyScore(db, 60)
	assert.NoError(t, err)
	assert.Len(t, maxScoreDS, 2)

	var rangeScoreDS Datasources
	err = rangeScoreDS.GetByPrivacyScoreRange(db, 40, 80)
	assert.NoError(t, err)
	assert.Len(t, rangeScoreDS, 2)
}

func TestDatasources_GetByUserID(t *testing.T) {
	db := setupTestDB(t)

	datasources := []Datasource{
		{Name: "DS1", UserID: 1},
		{Name: "DS2", UserID: 1},
		{Name: "DS3", UserID: 2},
		{Name: "DS4", UserID: 2},
	}
	for _, ds := range datasources {
		err := db.Create(&ds).Error
		assert.NoError(t, err)
	}

	var userDS Datasources
	err := userDS.GetByUserID(db, 1)
	assert.NoError(t, err)
	assert.Len(t, userDS, 2)
	assert.Equal(t, "DS1", userDS[0].Name)
	assert.Equal(t, "DS2", userDS[1].Name)
}
