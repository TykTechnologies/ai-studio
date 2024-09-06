package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTag_NewTag(t *testing.T) {
	tag := NewTag()
	assert.NotNil(t, tag)
}

func TestTag_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Test Create
	tag := &Tag{Name: "Test Tag"}
	err := tag.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, tag.ID)

	// Test Get
	fetchedTag := NewTag()
	err = fetchedTag.Get(db, tag.ID)
	assert.NoError(t, err)
	assert.Equal(t, tag.Name, fetchedTag.Name)

	// Test Update
	tag.Name = "Updated Test Tag"
	err = tag.Update(db)
	assert.NoError(t, err)

	err = fetchedTag.Get(db, tag.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Test Tag", fetchedTag.Name)

	// Test Delete
	err = tag.Delete(db)
	assert.NoError(t, err)

	err = fetchedTag.Get(db, tag.ID)
	assert.Error(t, err) // Should return an error as the tag is deleted
}

func TestTags_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test tags
	tags := []Tag{
		{Name: "Tag 1"},
		{Name: "Tag 2"},
		{Name: "Tag 3"},
	}
	for _, tag := range tags {
		err := db.Create(&tag).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedTags Tags
	err := fetchedTags.GetAll(db)
	assert.NoError(t, err)
	assert.Len(t, fetchedTags, 3)
	assert.Equal(t, "Tag 1", fetchedTags[0].Name)
	assert.Equal(t, "Tag 2", fetchedTags[1].Name)
	assert.Equal(t, "Tag 3", fetchedTags[2].Name)
}

func TestTags_GetByNameStub(t *testing.T) {
	db := setupTestDB(t)

	// Create some test tags
	tags := []Tag{
		{Name: "Apple"},
		{Name: "Banana"},
		{Name: "Cherry"},
		{Name: "Apricot"},
	}
	for _, tag := range tags {
		err := db.Create(&tag).Error
		assert.NoError(t, err)
	}

	// Test GetByNameStub
	var fetchedTags Tags
	err := fetchedTags.GetByNameStub(db, "A")
	assert.NoError(t, err)
	assert.Len(t, fetchedTags, 2)
	assert.Equal(t, "Apple", fetchedTags[0].Name)
	assert.Equal(t, "Apricot", fetchedTags[1].Name)

	// Test with a different stub
	fetchedTags = Tags{}
	err = fetchedTags.GetByNameStub(db, "B")
	assert.NoError(t, err)
	assert.Len(t, fetchedTags, 1)
	assert.Equal(t, "Banana", fetchedTags[0].Name)

	// Test with a stub that doesn't match any tags
	fetchedTags = Tags{}
	err = fetchedTags.GetByNameStub(db, "Z")
	assert.NoError(t, err)
	assert.Len(t, fetchedTags, 0)
}

func TestTag_GetByName(t *testing.T) {
	db := setupTestDB(t)

	// Create a test tag
	tag := &Tag{Name: "Unique Tag"}
	err := tag.Create(db)
	assert.NoError(t, err)

	// Test GetByName
	fetchedTag := NewTag()
	err = fetchedTag.GetByName(db, "Unique Tag")
	assert.NoError(t, err)
	assert.Equal(t, tag.ID, fetchedTag.ID)
	assert.Equal(t, tag.Name, fetchedTag.Name)

	// Test with a non-existent name
	nonExistentTag := NewTag()
	err = nonExistentTag.GetByName(db, "Non-existent Tag")
	assert.Error(t, err)
}
