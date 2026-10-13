package services

import (
	"github.com/TykTechnologies/midsommar/v2/data_session"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
)

func setupVendorTest(t *testing.T) *Service {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return NewService(db)
}

func TestGetAvailableLLMDrivers(t *testing.T) {
	service := setupVendorTest(t)

	drivers, err := service.GetAvailableLLMDrivers()
	assert.NoError(t, err)
	assert.NotEmpty(t, drivers, "Should return at least one LLM driver")

	// Verify driver structure
	for _, driver := range drivers {
		assert.NotEmpty(t, driver.Name, "Driver name should not be empty")
		assert.NotEmpty(t, driver.Vendor, "Driver vendor should not be empty")
		assert.NotEmpty(t, driver.Version, "Driver version should not be empty")
		assert.NotEmpty(t, driver.Description, "Driver description should not be empty")
		assert.NotEmpty(t, driver.SupportedFeatures, "Driver should have supported features")
	}

	// Check for expected vendors
	vendorNames := make(map[string]bool)
	for _, driver := range drivers {
		vendorNames[driver.Vendor] = true
	}

	// Should include common vendors (if enabled in switches)
	assert.True(t, len(vendorNames) > 0, "Should have at least one vendor")
}

func TestGetAvailableEmbedders(t *testing.T) {
	service := setupVendorTest(t)

	embedders, err := service.GetAvailableEmbedders()
	assert.NoError(t, err)
	assert.NotEmpty(t, embedders, "Should return at least one embedder")

	// Verify embedder structure
	for _, embedder := range embedders {
		assert.NotEmpty(t, embedder.Name, "Embedder name should not be empty")
		assert.NotEmpty(t, embedder.Vendor, "Embedder vendor should not be empty")
		assert.NotEmpty(t, embedder.Version, "Embedder version should not be empty")
		assert.NotEmpty(t, embedder.Description, "Embedder description should not be empty")
		assert.NotEmpty(t, embedder.SupportedFeatures, "Embedder should have supported features")

		// Embedder names should include "-embeddings" suffix
		assert.Contains(t, embedder.Name, "-embeddings")
	}
}

func TestGetAvailableVectorStores(t *testing.T) {
	service := setupVendorTest(t)

	vectorStores, err := service.GetAvailableVectorStores()
	assert.NoError(t, err)
	assert.NotEmpty(t, vectorStores, "Should return at least one vector store")

	// Verify vector store structure
	for _, store := range vectorStores {
		assert.NotEmpty(t, store.Name, "Vector store name should not be empty")
		assert.NotEmpty(t, store.Vendor, "Vector store vendor should not be empty")
		assert.NotEmpty(t, store.Version, "Vector store version should not be empty")
		assert.NotEmpty(t, store.Description, "Vector store description should not be empty")
		assert.NotEmpty(t, store.SupportedFeatures, "Vector store should have supported features")
	}

	// The list is exactly the stores the datasource code implements in this
	// build: offering one it does not (milvus) fails at use, and leaving one
	// out (pgvector, redis) hides it.
	var got, want []string
	for _, store := range vectorStores {
		got = append(got, store.Vendor)
	}
	for _, v := range data_session.AVAILABLE_VECTOR_STORES {
		want = append(want, string(v))
	}
	assert.ElementsMatch(t, want, got, "GetAvailableVectorStores drifted from data_session.AVAILABLE_VECTOR_STORES")
}
