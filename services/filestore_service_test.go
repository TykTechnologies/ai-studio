package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFilestoreTest(t *testing.T) (*Service, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)
	return service, db
}

func TestCreateFileStore(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	fileStore, err := service.CreateFileStore("test.txt", "Test file", "content here", 12)
	assert.NoError(t, err)
	assert.NotNil(t, fileStore)
	assert.Equal(t, "test.txt", fileStore.FileName)
	assert.Equal(t, "Test file", fileStore.Description)
	assert.Equal(t, "content here", fileStore.Content)
	assert.Equal(t, 12, fileStore.Length)
	assert.NotZero(t, fileStore.ID)
}

func TestUpdateFileStore(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	// Create initial filestore
	fileStore, err := service.CreateFileStore("original.txt", "Original", "original content", 16)
	assert.NoError(t, err)

	// Update it
	updated, err := service.UpdateFileStore(fileStore.ID, "updated.txt", "Updated desc", "new content", 11)
	assert.NoError(t, err)
	assert.Equal(t, "updated.txt", updated.FileName)
	assert.Equal(t, "Updated desc", updated.Description)
	assert.Equal(t, "new content", updated.Content)
	assert.Equal(t, 11, updated.Length)
	assert.Equal(t, fileStore.ID, updated.ID)
}

func TestGetFileStoreByID(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	// Create filestore
	created, err := service.CreateFileStore("get-test.txt", "Test", "content", 7)
	assert.NoError(t, err)

	t.Run("Get existing filestore", func(t *testing.T) {
		retrieved, err := service.GetFileStoreByID(created.ID)
		assert.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, "get-test.txt", retrieved.FileName)
	})

	t.Run("Get non-existent filestore", func(t *testing.T) {
		retrieved, err := service.GetFileStoreByID(99999)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestDeleteFileStore(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	// Create filestore
	fileStore, err := service.CreateFileStore("delete-test.txt", "Test", "content", 7)
	assert.NoError(t, err)

	t.Run("Delete existing filestore", func(t *testing.T) {
		err := service.DeleteFileStore(fileStore.ID)
		assert.NoError(t, err)

		// Verify it's deleted
		_, err = service.GetFileStoreByID(fileStore.ID)
		assert.Error(t, err)
	})

	t.Run("Delete non-existent filestore", func(t *testing.T) {
		err := service.DeleteFileStore(99999)
		assert.Error(t, err)
	})
}

func TestGetFileStoreByFileName(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	// Create filestore
	created, err := service.CreateFileStore("unique-name.txt", "Test", "content", 7)
	assert.NoError(t, err)

	t.Run("Get by existing filename", func(t *testing.T) {
		retrieved, err := service.GetFileStoreByFileName("unique-name.txt")
		assert.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, "unique-name.txt", retrieved.FileName)
	})

	t.Run("Get by non-existent filename", func(t *testing.T) {
		retrieved, err := service.GetFileStoreByFileName("non-existent.txt")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestGetAllFileStores(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	// Create multiple filestores
	for i := 1; i <= 5; i++ {
		_, err := service.CreateFileStore("file"+string(rune('0'+i))+".txt", "Test", "content", 7)
		assert.NoError(t, err)
	}

	t.Run("Get all with pagination", func(t *testing.T) {
		stores, totalCount, totalPages, err := service.GetAllFileStores(2, 1, false)
		assert.NoError(t, err)
		assert.Len(t, stores, 2, "Should return page size of 2")
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages) // 5 items / 2 per page = 3 pages
	})

	t.Run("Get all without pagination", func(t *testing.T) {
		// Use large page size to get all
		stores, totalCount, totalPages, err := service.GetAllFileStores(100, 1, true)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(stores), 5)
		assert.Equal(t, int64(5), totalCount)
		assert.GreaterOrEqual(t, totalPages, 1)
	})
}

func TestSearchFileStores(t *testing.T) {
	service, _ := setupFilestoreTest(t)

	// Create filestores with different names
	_, err := service.CreateFileStore("report.pdf", "Annual Report", "content", 7)
	assert.NoError(t, err)
	_, err = service.CreateFileStore("invoice.pdf", "Invoice", "content", 7)
	assert.NoError(t, err)
	_, err = service.CreateFileStore("summary.txt", "Summary", "content", 7)
	assert.NoError(t, err)

	t.Run("Search finds matching files", func(t *testing.T) {
		results, err := service.SearchFileStores("pdf")
		assert.NoError(t, err)
		assert.Len(t, results, 2, "Should find 2 PDF files")
	})

	t.Run("Search by description", func(t *testing.T) {
		results, err := service.SearchFileStores("Report")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)
	})

	t.Run("Search with no matches", func(t *testing.T) {
		results, err := service.SearchFileStores("nonexistent")
		assert.NoError(t, err)
		assert.Len(t, results, 0)
	})
}
