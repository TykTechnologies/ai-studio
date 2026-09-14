package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupListQueryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	return db
}

func TestLikePattern_EscapesWildcards(t *testing.T) {
	assert.Equal(t, `%abc%`, likePattern("abc"))
	assert.Equal(t, `%50\%%`, likePattern("50%"), "percent is a literal")
	assert.Equal(t, `%a\_b%`, likePattern("a_b"), "underscore is a literal")
	assert.Equal(t, `%c:\\dir%`, likePattern(`c:\dir`), "the escape character itself is escaped")
	assert.Equal(t, `%mixedcase%`, likePattern("MixedCase"), "lower-cased to match LOWER(col)")
}

func TestApplySearch_SubstringCaseInsensitiveAndEscaped(t *testing.T) {
	db := setupListQueryDB(t)
	for _, l := range []models.LLM{
		{Name: "GPT-4 Turbo", ShortDescription: "OpenAI flagship", Vendor: models.OPENAI},
		{Name: "claude", ShortDescription: "100% Anthropic", Vendor: models.ANTHROPIC},
		{Name: "under_score", ShortDescription: "", Vendor: models.MOCK_VENDOR},
		{Name: "mock-a", ShortDescription: "", Vendor: models.MOCK_VENDOR},
	} {
		require.NoError(t, db.Create(&l).Error)
	}
	cols := []string{"name", "short_description", "vendor"}

	names := func(term string) []string {
		var llms []models.LLM
		require.NoError(t, applySearch(db.Model(&models.LLM{}), cols, term).Order("id").Find(&llms).Error)
		out := []string{}
		for _, l := range llms {
			out = append(out, l.Name)
		}
		return out
	}

	assert.Equal(t, []string{"GPT-4 Turbo"}, names("gpt-4"), "case-insensitive on name")
	assert.Equal(t, []string{"claude"}, names("ANTHROPIC"), "matches the vendor column")
	assert.Equal(t, []string{"GPT-4 Turbo"}, names("flagship"), "matches the description column")
	assert.Equal(t, []string{"claude"}, names("100%"), "a literal percent is not a wildcard")
	assert.Equal(t, []string{"under_score"}, names("r_s"), "a literal underscore is not a wildcard")
	assert.Equal(t, []string{"under_score", "mock-a"}, names("mock"), "vendor mock matches both")
	assert.Len(t, names("   "), 4, "blank term is no filter")
}

func TestApplySort_DirectionAndTiebreak(t *testing.T) {
	db := setupListQueryDB(t)
	for _, l := range []models.LLM{
		{Name: "b", PrivacyScore: 50},
		{Name: "a", PrivacyScore: 50},
		{Name: "c", PrivacyScore: 10},
	} {
		require.NoError(t, db.Create(&l).Error)
	}
	names := func(col string, desc bool) []string {
		var llms []models.LLM
		require.NoError(t, applySort(db.Model(&models.LLM{}), col, desc).Find(&llms).Error)
		out := []string{}
		for _, l := range llms {
			out = append(out, l.Name)
		}
		return out
	}
	assert.Equal(t, []string{"a", "b", "c"}, names("name", false))
	assert.Equal(t, []string{"c", "b", "a"}, names("name", true))
	// Equal privacy scores fall back to id ascending (b was created first).
	assert.Equal(t, []string{"c", "b", "a"}, names("privacy_score", false))
	assert.Equal(t, []string{"b", "a", "c"}, names("privacy_score", true))
}

func TestListOptions_ThroughGetAll(t *testing.T) {
	db := setupListQueryDB(t)
	s := NewService(db)
	for _, l := range []models.LLM{
		{Name: "mock-b", Vendor: models.MOCK_VENDOR},
		{Name: "mock-a", Vendor: models.MOCK_VENDOR},
		{Name: "real", Vendor: models.OPENAI},
	} {
		require.NoError(t, db.Create(&l).Error)
	}

	llms, total, pages, err := s.GetAllLLMs(10, 1, false, ListOptions{Search: "mock", Sort: "name", SortDesc: true})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total, "the count reflects the filtered set")
	assert.Equal(t, 1, pages)
	require.Len(t, llms, 2)
	assert.Equal(t, "mock-b", llms[0].Name)
	assert.Equal(t, "mock-a", llms[1].Name)

	// No options: the pre-existing behaviour, id ascending, everything.
	llms, total, _, err = s.GetAllLLMs(10, 1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, "mock-b", llms[0].Name)

	// Page size 1 with a filter: X-Total-Pages counts filtered rows.
	_, total, pages, err = s.GetAllLLMs(1, 1, false, ListOptions{Search: "mock"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, 2, pages)

	filters, _, _, err := s.ListFilters(10, 1, true, ListOptions{Search: "nothing"})
	require.NoError(t, err)
	assert.Empty(t, filters)
}

func TestGetCatalogueGroups(t *testing.T) {
	db := setupListQueryDB(t)
	s := NewService(db)

	cat := &models.Catalogue{Name: "Shared"}
	require.NoError(t, db.Create(cat).Error)
	lonely := &models.Catalogue{Name: "Lonely"}
	require.NoError(t, db.Create(lonely).Error)
	dcat := &models.DataCatalogue{Name: "Docs"}
	require.NoError(t, db.Create(dcat).Error)
	tcat := &models.ToolCatalogue{Name: "Tools"}
	require.NoError(t, db.Create(tcat).Error)

	platform := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(platform).Error)
	analytics := &models.Group{Name: "Analytics"}
	require.NoError(t, db.Create(analytics).Error)

	u1 := &models.User{Email: "one@example.com", Name: "One", Password: "x"}
	u2 := &models.User{Email: "two@example.com", Name: "Two", Password: "x"}
	require.NoError(t, db.Create(u1).Error)
	require.NoError(t, db.Create(u2).Error)
	require.NoError(t, db.Model(platform).Association("Users").Append(u1, u2))
	require.NoError(t, db.Model(analytics).Association("Users").Append(u1))

	require.NoError(t, db.Model(platform).Association("Catalogues").Append(cat))
	require.NoError(t, db.Model(analytics).Association("Catalogues").Append(cat))
	require.NoError(t, db.Model(platform).Association("DataCatalogues").Append(dcat))
	require.NoError(t, db.Model(analytics).Association("ToolCatalogues").Append(tcat))

	groups, err := s.GetCatalogueGroups("catalogues", cat.ID)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "Analytics", groups[0].Name, "ordered by name")
	assert.Equal(t, int64(1), groups[0].MemberCount)
	assert.Equal(t, "Platform", groups[1].Name)
	assert.Equal(t, int64(2), groups[1].MemberCount)

	// A soft-deleted member is not counted.
	require.NoError(t, db.Delete(u2).Error)
	groups, err = s.GetCatalogueGroups("catalogues", cat.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), groups[1].MemberCount)

	groups, err = s.GetCatalogueGroups("catalogues", lonely.ID)
	require.NoError(t, err)
	assert.NotNil(t, groups)
	assert.Empty(t, groups, "empty, never nil")

	groups, err = s.GetCatalogueGroups("data-catalogues", dcat.ID)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "Platform", groups[0].Name)

	groups, err = s.GetCatalogueGroups("tool-catalogues", tcat.ID)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "Analytics", groups[0].Name)

	_, err = s.GetCatalogueGroups("bogus", cat.ID)
	assert.Error(t, err)
}

func TestGetAppDependents(t *testing.T) {
	db := setupListQueryDB(t)
	s := NewService(db)

	app := &models.App{Name: "Copilot", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	other := &models.App{Name: "Other", UserID: 1}
	require.NoError(t, db.Create(other).Error)
	plugin := &models.Plugin{Name: "agent-plugin", Command: "/bin/true", HookType: "agent"}
	require.NoError(t, db.Create(plugin).Error)
	agent := &models.AgentConfig{Name: "Helper", Slug: "helper", PluginID: plugin.ID, AppID: app.ID}
	require.NoError(t, db.Create(agent).Error)

	deps, err := s.GetAppDependents(app.ID)
	require.NoError(t, err)
	require.Len(t, deps.Agents, 1)
	assert.Equal(t, "Helper", deps.Agents[0].Name)
	assert.Equal(t, agent.ID, deps.Agents[0].ID)
	assert.Equal(t, 1, deps.Total)
	assert.NotNil(t, deps.Chats)
	assert.Empty(t, deps.Chats)

	deps, err = s.GetAppDependents(other.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, deps.Total)
	assert.NotNil(t, deps.Agents)
}
