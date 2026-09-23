package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// routerFixture: a user in a team holding catalogue "LLMs"; LLMs "Public"
// (privacy 10) and "Private" (privacy 80); Model Router "prod" routing to
// both (so it scores 10) and published in the catalogue; router "hidden" not
// published anywhere; a datasource of privacy 50.
type routerFixture struct {
	svc             *Service
	db              *gorm.DB
	user            *models.User
	public, private *models.LLM
	prod, hidden    *models.ModelRouter
	catalogue       *models.Catalogue
	datasource      *models.Datasource
}

func newRouterFixture(t *testing.T) *routerFixture {
	db := setupTestDB(t)
	svc := NewService(db)
	f := &routerFixture{svc: svc, db: db}

	user, err := svc.CreateUser(UserDTO{Email: "dev@example.com", Name: "dev", Password: "password123"})
	require.NoError(t, err)
	f.user = user

	f.public = &models.LLM{Name: "Public", Vendor: models.OPENAI, Active: true, PrivacyScore: 10}
	f.private = &models.LLM{Name: "Private", Vendor: models.OPENAI, Active: true, PrivacyScore: 80}
	require.NoError(t, db.Create(f.public).Error)
	require.NoError(t, db.Create(f.private).Error)

	mkRouter := func(name, slug string) *models.ModelRouter {
		r := &models.ModelRouter{Name: name, Slug: slug, Active: true, Pools: []*models.ModelPool{{
			Name: "all", ModelPattern: "gpt-4o,claude-*", SelectionAlgorithm: models.SelectionRoundRobin,
			Vendors: []*models.PoolVendor{
				{LLMID: f.public.ID, Weight: 1, Active: true},
				{LLMID: f.private.ID, Weight: 1, Active: true, Mappings: []*models.ModelMapping{{SourceModel: "best", TargetModel: "gpt-4o"}}},
			},
		}}}
		require.NoError(t, r.Create(db))
		return r
	}
	f.prod = mkRouter("Prod", "prod")
	f.hidden = mkRouter("Hidden", "hidden")

	f.catalogue = &models.Catalogue{Name: "LLMs"}
	require.NoError(t, db.Create(f.catalogue).Error)
	_, err = svc.SetModelRouterCatalogues(f.prod.ID, []uint{f.catalogue.ID})
	require.NoError(t, err)
	group, err := svc.CreateGroup("Team", []uint{user.ID}, []uint{f.catalogue.ID}, nil, nil)
	require.NoError(t, err)
	_ = group

	f.datasource = &models.Datasource{Name: "Docs", PrivacyScore: 50, Active: true}
	require.NoError(t, db.Create(f.datasource).Error)
	return f
}

func TestValidateModelRouterBindings_Visibility(t *testing.T) {
	f := newRouterFixture(t)

	got, err := f.svc.ValidateModelRouterBindings(f.user.ID, false, []uint{f.prod.ID})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "prod", got[0].Slug)

	_, err = f.svc.ValidateModelRouterBindings(f.user.ID, false, []uint{f.hidden.ID})
	assert.ErrorIs(t, err, ErrModelRouterNotVisible, "not in any of the user's catalogues")

	_, err = f.svc.ValidateModelRouterBindings(f.user.ID, true, []uint{f.hidden.ID})
	assert.NoError(t, err, "an administrator may grant any active router")

	require.NoError(t, f.db.Model(f.prod).Update("active", false).Error)
	_, err = f.svc.ValidateModelRouterBindings(f.user.ID, false, []uint{f.prod.ID})
	assert.ErrorIs(t, err, ErrModelRouterNotVisible, "an inactive router is not grantable")
}

func TestModelRouterPrivacyScores_LowestReachableLLM(t *testing.T) {
	f := newRouterFixture(t)
	scores, err := models.ModelRouterPrivacyScores(f.db, []uint{f.prod.ID})
	require.NoError(t, err)
	assert.Equal(t, 10, scores[f.prod.ID])

	// Deactivating the less private LLM raises the router's score.
	require.NoError(t, f.db.Model(f.public).Update("active", false).Error)
	scores, err = models.ModelRouterPrivacyScores(f.db, []uint{f.prod.ID})
	require.NoError(t, err)
	assert.Equal(t, 80, scores[f.prod.ID])

	llms, err := f.svc.ModelRouterReachableLLMs(f.prod.ID)
	require.NoError(t, err)
	require.Len(t, llms, 1)
	assert.Equal(t, "Private", llms[0].Name)
}

func TestCreateApp_WithModelRouters(t *testing.T) {
	f := newRouterFixture(t)

	// A router is a provider for the privacy rule, scored by its least private
	// LLM (10): a privacy-50 datasource may not be sent through it.
	_, err := f.svc.CreateApp("leaky", "", f.user.ID, []uint{f.datasource.ID}, nil, nil, nil, nil, nil, WithModelRouters([]uint{f.prod.ID}))
	var mismatch *PrivacyScoreMismatch
	require.True(t, errors.As(err, &mismatch), "got %v", err)
	assert.Equal(t, "Prod", mismatch.LLMName)

	app, err := f.svc.CreateApp("router app", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithModelRouters([]uint{f.prod.ID}))
	require.NoError(t, err)
	app, err = f.svc.GetAppByID(app.ID)
	require.NoError(t, err)
	require.Len(t, app.ModelRouters, 1)
	assert.Equal(t, f.prod.ID, app.ModelRouters[0].ID)
	assert.Empty(t, app.LLMs, "a router grant is not a grant of its LLMs")

	// An update that says nothing about routers keeps them...
	app, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, app.ModelRouters, 1)
	// ...and still counts them in the privacy check.
	_, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, []uint{f.datasource.ID}, nil, nil, nil, nil, nil)
	assert.True(t, errors.As(err, &mismatch))

	// An explicit empty list clears them.
	app, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithModelRouters(nil))
	require.NoError(t, err)
	assert.Empty(t, app.ModelRouters)
}

func TestDeleteApp_ClearsRouterGrants(t *testing.T) {
	f := newRouterFixture(t)
	app, err := f.svc.CreateApp("router app", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithModelRouters([]uint{f.prod.ID}))
	require.NoError(t, err)
	require.NoError(t, f.svc.DeleteApp(app.ID))
	var n int64
	require.NoError(t, f.db.Table("app_model_routers").Where("app_id = ?", app.ID).Count(&n).Error)
	assert.Zero(t, n)
}

func TestDeleteModelRouter_WithdrawsGrantsAndPublication(t *testing.T) {
	f := newRouterFixture(t)
	_, err := f.svc.CreateApp("router app", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithModelRouters([]uint{f.prod.ID}))
	require.NoError(t, err)

	require.NoError(t, f.prod.Delete(f.db))
	for _, table := range []string{"app_model_routers", "catalogue_model_routers"} {
		var n int64
		require.NoError(t, f.db.Table(table).Where("model_router_id = ?", f.prod.ID).Count(&n).Error)
		assert.Zero(t, n, table)
	}
}

func TestSetModelRouterCatalogues(t *testing.T) {
	f := newRouterFixture(t)
	cats, err := f.svc.GetModelRouterCatalogues(f.prod.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)

	_, err = f.svc.SetModelRouterCatalogues(f.prod.ID, []uint{f.catalogue.ID, 9999})
	assert.ErrorIs(t, err, ErrInvalidCatalogueReference)

	_, err = f.svc.SetModelRouterCatalogues(f.prod.ID, nil)
	require.NoError(t, err)
	_, err = f.svc.ValidateModelRouterBindings(f.user.ID, false, []uint{f.prod.ID})
	assert.ErrorIs(t, err, ErrModelRouterNotVisible, "withdrawn from the catalogue")
}

func TestAdvertisedModels(t *testing.T) {
	f := newRouterFixture(t)
	var r models.ModelRouter
	require.NoError(t, r.Get(f.db, f.prod.ID))
	assert.Equal(t, []string{"best", "gpt-4o"}, r.AdvertisedModels())
}

func TestRouteSlugs_LLMsAndRoutersDoNotClash(t *testing.T) {
	f := newRouterFixture(t)

	// An LLM named so that it answers to "prod" would shadow the router.
	_, err := f.svc.CreateLLM("Prod", "", "", 0, "", "", "", models.OPENAI, true, nil, "", nil, nil, nil, false, nil, nil)
	assert.ErrorIs(t, err, models.ErrRouteSlugTaken)

	// Renaming an LLM onto a router's slug is refused too.
	_, err = f.svc.UpdateLLM(f.public.ID, "PROD", "", "", 10, "", "", "", models.OPENAI, true, nil, "", nil, nil, nil, "", false, nil, nil)
	assert.ErrorIs(t, err, models.ErrRouteSlugTaken)

	// A router slug an LLM already answers to is refused.
	assert.ErrorIs(t, models.CheckRouterRouteSlug(f.db, "public"), models.ErrRouteSlugTaken)
	assert.NoError(t, models.CheckRouterRouteSlug(f.db, "something-else"))
}
