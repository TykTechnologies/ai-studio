package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// semanticFixture extends routerFixture with LLM "Judge" (privacy 5) and
// Semantic Router "smart": complex -> Private, simple -> Model Router prod
// (Public and Private), judged by Judge; published in the team's catalogue.
// Router "hidden-smart" is published nowhere.
type semanticFixture struct {
	*routerFixture
	judge         *models.LLM
	smart, hidden *models.SemanticRouter
}

func newSemanticFixture(t *testing.T) *semanticFixture {
	f := &semanticFixture{routerFixture: newRouterFixture(t)}
	f.judge = &models.LLM{Name: "Judge", Vendor: models.OPENAI, Active: true, PrivacyScore: 5}
	require.NoError(t, f.db.Create(f.judge).Error)
	mk := func(name, slug string) *models.SemanticRouter {
		r := &models.SemanticRouter{Name: name, Slug: slug, Active: true,
			Settings: sr.Settings{DefaultRoute: "simple",
				Judge: sr.JudgeSettings{Enabled: true, ModelRef: sr.ModelRef{LLMID: f.judge.ID, Model: "m"}}},
			Routes: []sr.Route{
				{Name: "complex", Target: sr.Target{Type: sr.TargetLLM, LLMID: f.private.ID, Model: "big"}},
				{Name: "simple", Target: sr.Target{Type: sr.TargetModelRouter, ModelRouterID: f.prod.ID, Model: "gpt-4o"}},
			}}
		require.NoError(t, r.Create(f.db))
		return r
	}
	f.smart = mk("Smart", "smart")
	f.hidden = mk("Hidden Smart", "hidden-smart")
	_, err := f.svc.SetSemanticRouterCatalogues(f.smart.ID, []uint{f.catalogue.ID})
	require.NoError(t, err)
	return f
}

func TestValidateSemanticRouterBindings_Visibility(t *testing.T) {
	f := newSemanticFixture(t)
	got, err := f.svc.ValidateSemanticRouterBindings(f.user.ID, false, []uint{f.smart.ID})
	require.NoError(t, err)
	require.Len(t, got, 1)

	_, err = f.svc.ValidateSemanticRouterBindings(f.user.ID, false, []uint{f.hidden.ID})
	assert.ErrorIs(t, err, ErrSemanticRouterNotVisible)
	_, err = f.svc.ValidateSemanticRouterBindings(f.user.ID, true, []uint{f.hidden.ID})
	assert.NoError(t, err, "an administrator may grant any active router")

	require.NoError(t, f.db.Model(f.smart).Update("active", false).Error)
	_, err = f.svc.ValidateSemanticRouterBindings(f.user.ID, true, []uint{f.smart.ID})
	assert.ErrorIs(t, err, ErrSemanticRouterNotVisible, "an inactive router cannot be granted")
}

func TestSemanticRouterPrivacy_CountsClassifiersAndHandOffs(t *testing.T) {
	f := newSemanticFixture(t)
	scores, err := models.SemanticRouterPrivacyScores(f.db, []uint{f.smart.ID})
	require.NoError(t, err)
	assert.Equal(t, 5, scores[f.smart.ID], "the judge sees the prompt too")

	llms, err := f.svc.SemanticRouterReachableLLMs(f.smart.ID)
	require.NoError(t, err)
	var names []string
	for _, l := range llms {
		names = append(names, l.Name)
	}
	assert.Equal(t, []string{"Private", "Public"}, names, "direct targets and the hand-off's vendors, not the judge")
}

func TestCreateApp_WithSemanticRouters(t *testing.T) {
	f := newSemanticFixture(t)

	_, err := f.svc.CreateApp("leaky", "", f.user.ID, []uint{f.datasource.ID}, nil, nil, nil, nil, nil, WithSemanticRouters([]uint{f.smart.ID}))
	var mismatch *PrivacyScoreMismatch
	require.True(t, errors.As(err, &mismatch), "got %v", err)

	app, err := f.svc.CreateApp("smart app", "", f.user.ID, nil, nil, nil, nil, nil, nil,
		WithSemanticRouters([]uint{f.smart.ID}), WithModelRouters([]uint{f.prod.ID}))
	require.NoError(t, err)
	app, err = f.svc.GetAppByID(app.ID)
	require.NoError(t, err)
	require.Len(t, app.SemanticRouters, 1)
	require.Len(t, app.ModelRouters, 1)

	// An update that leaves routers alone keeps both kinds and counts them.
	app, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, app.SemanticRouters, 1)
	_, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, []uint{f.datasource.ID}, nil, nil, nil, nil, nil, WithModelRouters(nil))
	assert.True(t, errors.As(err, &mismatch), "the kept semantic grant still counts")

	// Clearing one kind leaves the other.
	app, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithSemanticRouters(nil))
	require.NoError(t, err)
	assert.Empty(t, app.SemanticRouters)
	assert.Len(t, app.ModelRouters, 1)

	app, err = f.svc.UpdateApp(app.ID, "renamed", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithSemanticRouters([]uint{f.smart.ID}))
	require.NoError(t, err)
	require.NoError(t, f.svc.DeleteApp(app.ID))
	var n int64
	require.NoError(t, f.db.Table("app_semantic_routers").Where("app_id = ?", app.ID).Count(&n).Error)
	assert.Zero(t, n)
}

func TestSemanticRouterDependents(t *testing.T) {
	f := newSemanticFixture(t)
	_, err := f.svc.CreateApp("smart app", "", f.user.ID, nil, nil, nil, nil, nil, nil, WithSemanticRouters([]uint{f.smart.ID}))
	require.NoError(t, err)

	d, err := f.svc.GetSemanticRouterDependents(f.smart.ID)
	require.NoError(t, err)
	assert.Len(t, d.Apps, 1)
	assert.Len(t, d.Catalogues, 1)

	d, err = f.svc.GetLLMDependents(f.judge.ID)
	require.NoError(t, err)
	assert.Len(t, d.SemanticRouters, 2, "a classifier LLM is a dependency too")

	d, err = f.svc.GetModelRouterDependents(f.prod.ID)
	require.NoError(t, err)
	assert.Len(t, d.SemanticRouters, 2, "routers that hand off to it")
}

func TestRouteSlugs_SemanticRoutersClashWithLLMsAndModelRouters(t *testing.T) {
	f := newSemanticFixture(t)
	_, err := f.svc.CreateLLM("Smart", "", "", 0, "", "", "", models.OPENAI, true, nil, "", nil, nil, nil, false, nil, nil)
	assert.ErrorIs(t, err, models.ErrRouteSlugTaken, "an LLM may not shadow a Semantic Router")
	assert.ErrorIs(t, models.CheckRouterRouteSlug(f.db, "smart"), models.ErrRouteSlugTaken, "nor a Model Router")
	assert.ErrorIs(t, models.CheckSemanticRouterRouteSlug(f.db, "prod"), models.ErrRouteSlugTaken)
	assert.ErrorIs(t, models.CheckSemanticRouterRouteSlug(f.db, "public"), models.ErrRouteSlugTaken)
	assert.NoError(t, models.CheckSemanticRouterRouteSlug(f.db, "fresh"))
}
