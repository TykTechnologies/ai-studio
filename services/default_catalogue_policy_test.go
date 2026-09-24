package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultCatalogueAutoAdds is the number of catalogues a freshly created
// object lands in with no catalogue chosen: 1 (Default) in Community Edition,
// 0 in Enterprise builds. Tests that count catalogue memberships use it so
// they hold under both build tags.
func defaultCatalogueAutoAdds() int {
	if autoAddToDefaultCatalogue() {
		return 1
	}
	return 0
}

func TestAutoAddToDefaultCatalogue_FollowsEdition(t *testing.T) {
	assert.Equal(t, !group_access.IsFilteringEnabled(), autoAddToDefaultCatalogue(),
		"auto-add is the inverse of group filtering: CE has no other route to the portal, Enterprise uses catalogues as access control")
}

func TestCreateLLM_DefaultCatalogueIsEditionAware(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	llm, err := service.CreateLLM("Edition LLM", "api_key", "https://api.example.com",
		80, "Short", "Long", "", models.OPENAI, true, nil, "", []string{}, nil, nil, false, nil, nil)
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Table("catalogue_llms").Where("llm_id = ?", llm.ID).Count(&count).Error)

	if autoAddToDefaultCatalogue() {
		assert.EqualValues(t, 1, count, "CE: a new LLM joins the Default catalogue")
		defaultCatalogue, err := models.GetOrCreateDefaultCatalogue(db)
		require.NoError(t, err)
		var inDefault int64
		require.NoError(t, db.Table("catalogue_llms").
			Where("llm_id = ? AND catalogue_id = ?", llm.ID, defaultCatalogue.ID).Count(&inDefault).Error)
		assert.EqualValues(t, 1, inDefault)
	} else {
		assert.EqualValues(t, 0, count, "Enterprise: a new LLM is in no catalogue until an administrator grants it")
	}
}

func TestCreateDatasource_DefaultCatalogueIsEditionAware(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	ds, err := service.CreateDatasource("Edition DS", "Short", "Long", "icon.png", "https://example.com",
		75, 1, []string{}, "conn", "type", "key", "db", EmbedderInput{Vendor: "vendor", URL: "url", APIKey: "ekey", Model: "model"}, true)
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Table("data_catalogue_data_sources").Where("datasource_id = ?", ds.ID).Count(&count).Error)
	assert.EqualValues(t, defaultCatalogueAutoAdds(), count)
}

func TestCreateTool_DefaultCatalogueIsEditionAware(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	tool, err := service.CreateTool("Edition Tool", "Description", models.ToolTypeREST, "spec", 8, "apiKey", "secret")
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Table("tool_catalogue_tools").Where("tool_id = ?", tool.ID).Count(&count).Error)
	assert.EqualValues(t, defaultCatalogueAutoAdds(), count)
}

// The Default group and the Default catalogues exist in both editions; only
// the object-side auto-add is edition-specific.
func TestDefaultGroupAndCatalogueExistInBothEditions(t *testing.T) {
	db := setupTestDB(t)

	var group models.Group
	require.NoError(t, db.Where("name = ?", models.DefaultGroupName).First(&group).Error)

	catalogue, err := models.GetOrCreateDefaultCatalogue(db)
	require.NoError(t, err)
	assert.Equal(t, models.DefaultCatalogueName, catalogue.Name)
}
