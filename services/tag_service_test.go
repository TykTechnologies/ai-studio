package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForTags(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestTagService(t *testing.T) {
	db := setupTestDBForTags(t)
	service := NewService(db)

	// Test CreateTag
	tag, err := service.CreateTag("Test Tag")
	assert.NoError(t, err)
	assert.NotNil(t, tag)
	assert.NotZero(t, tag.ID)
	assert.Equal(t, "Test Tag", tag.Name)

	// Test GetTagByID
	fetchedTag, err := service.GetTagByID(tag.ID)
	assert.NoError(t, err)
	assert.Equal(t, tag.ID, fetchedTag.ID)
	assert.Equal(t, tag.Name, fetchedTag.Name)

	// Test UpdateTag
	updatedTag, err := service.UpdateTag(tag.ID, "Updated Tag")
	assert.NoError(t, err)
	assert.Equal(t, tag.ID, updatedTag.ID)
	assert.Equal(t, "Updated Tag", updatedTag.Name)

	// Test GetAllTags
	allTags, err := service.GetAllTags()
	assert.NoError(t, err)
	assert.Len(t, allTags, 1)
	assert.Equal(t, updatedTag.ID, allTags[0].ID)
	assert.Equal(t, updatedTag.Name, allTags[0].Name)

	// Test SearchTagsByNameStub
	searchedTags, err := service.SearchTagsByNameStub("Upd")
	assert.NoError(t, err)
	assert.Len(t, searchedTags, 1)
	assert.Equal(t, updatedTag.ID, searchedTags[0].ID)
	assert.Equal(t, updatedTag.Name, searchedTags[0].Name)

	// Test GetTagByName
	tagByName, err := service.GetTagByName("Updated Tag")
	assert.NoError(t, err)
	assert.Equal(t, updatedTag.ID, tagByName.ID)
	assert.Equal(t, updatedTag.Name, tagByName.Name)

	// Test DeleteTag
	err = service.DeleteTag(tag.ID)
	assert.NoError(t, err)

	// Verify tag is deleted
	_, err = service.GetTagByID(tag.ID)
	assert.Error(t, err)
}

func TestTagService_MultipleTagsScenario(t *testing.T) {
	db := setupTestDBForTags(t)
	service := NewService(db)

	// Create multiple tags
	tag1, _ := service.CreateTag("AI")
	tag2, _ := service.CreateTag("Machine Learning")
	tag3, _ := service.CreateTag("Natural Language Processing")

	// Test GetAllTags
	allTags, err := service.GetAllTags()
	assert.NoError(t, err)
	assert.Len(t, allTags, 3)

	// Test SearchTagsByNameStub
	aiTags, err := service.SearchTagsByNameStub("AI")
	assert.NoError(t, err)
	assert.Len(t, aiTags, 1)
	assert.Equal(t, tag1.ID, aiTags[0].ID)

	mlTags, err := service.SearchTagsByNameStub("Machine")
	assert.NoError(t, err)
	assert.Len(t, mlTags, 1)
	assert.Equal(t, tag2.ID, mlTags[0].ID)

	// Test GetTagByName
	nlpTag, err := service.GetTagByName("Natural Language Processing")
	assert.NoError(t, err)
	assert.Equal(t, tag3.ID, nlpTag.ID)

	// Test non-existent tag
	nonExistentTag, err := service.GetTagByName("Non-existent Tag")
	assert.Error(t, err)
	assert.Nil(t, nonExistentTag)
}
