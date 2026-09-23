package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Model Routers as App assets.
//
// A router is published like an LLM: it sits in LLM catalogues, teams are
// granted the catalogues, and an App is granted the router. The grant lets the
// App reach every LLM the router can pick, but only through the router (the
// gateway enforces that; see proxy/router.go). For the privacy rule a router
// is a provider like an LLM, scored by the least private LLM it can reach:
// whatever an App sends through the router may end up there.

// ErrModelRouterNotVisible is returned when an App would be granted a Model
// Router the caller's teams cannot see, or one that is not active.
var ErrModelRouterNotVisible = errors.New("model router is not available to this app")

// AppOption adjusts an App create or update.
type AppOption func(*appOptions)

type appOptions struct {
	// modelRouterIDs replaces the App's router grants when set; nil leaves
	// them as they are (an update) or empty (a create).
	modelRouterIDs *[]uint
}

func resolveAppOptions(opts []AppOption) appOptions {
	var o appOptions
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return o
}

// WithModelRouters sets the App's Model Router grants. Callers check
// visibility first (ValidateModelRouterBindings).
func WithModelRouters(ids []uint) AppOption {
	return func(o *appOptions) {
		cp := uniqueUintIDs(ids)
		o.modelRouterIDs = &cp
	}
}

// ValidateModelRouterBindings checks that every router may be granted by this
// user: active, and in an LLM catalogue one of the user's teams holds (an
// administrator may grant any active router). It returns the routers in the
// order given.
func (s *Service) ValidateModelRouterBindings(userID uint, isAdmin bool, routerIDs []uint) ([]models.ModelRouter, error) {
	ids := uniqueUintIDs(routerIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	q := s.DB.Model(&models.ModelRouter{})
	if isAdmin {
		q = q.Where("model_routers.active = ?", true)
	} else {
		q = models.AccessibleModelRouterQuery(s.DB, userID)
	}
	var routers []models.ModelRouter
	if err := q.Where("model_routers.id IN ?", ids).Group("model_routers.id").Find(&routers).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]models.ModelRouter, len(routers))
	for _, r := range routers {
		byID[r.ID] = r
	}
	out := make([]models.ModelRouter, 0, len(ids))
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: router %d", ErrModelRouterNotVisible, id)
		}
		out = append(out, r)
	}
	return out, nil
}

// ModelRouterReachableLLMs lists the active LLMs behind a router's active
// vendors: what a grant of the router lets an App reach.
func (s *Service) ModelRouterReachableLLMs(routerID uint) ([]models.LLM, error) {
	var llms []models.LLM
	err := s.DB.Model(&models.LLM{}).
		Joins("JOIN pool_vendors ON pool_vendors.llm_id = llms.id AND pool_vendors.deleted_at IS NULL").
		Joins("JOIN model_pools ON model_pools.id = pool_vendors.pool_id AND model_pools.deleted_at IS NULL").
		Where("model_pools.router_id = ? AND pool_vendors.active = ? AND llms.active = ?", routerID, true, true).
		Group("llms.id").
		Order("llms.name").
		Find(&llms).Error
	return llms, err
}

// modelRouterProviders scores routers as providers for the privacy rule, in
// one query: each router at the lowest privacy score among the LLMs it can
// reach (models.ModelRouterPrivacySQL). A router that reaches none is left
// out; it cannot carry anything anywhere.
func (s *Service) modelRouterProviders(routerIDs []uint) ([]privacyResource, error) {
	ids := uniqueUintIDs(routerIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	var routers []models.ModelRouter
	if err := s.DB.Select("id", "name").Where("id IN ?", ids).Find(&routers).Error; err != nil {
		return nil, err
	}
	if len(routers) != len(ids) {
		return nil, fmt.Errorf("%w: one or more model routers do not exist", ErrModelRouterNotVisible)
	}
	scores, err := models.ModelRouterPrivacyScores(s.DB, ids)
	if err != nil {
		return nil, err
	}
	var out []privacyResource
	for _, r := range routers {
		if score, ok := scores[r.ID]; ok {
			out = append(out, privacyResource{Name: r.Name, Score: score})
		}
	}
	return out, nil
}

// routerProvidersFor is the router side of an App's privacy check: the
// routers the change sets, or, when it leaves them alone, the ones the App
// already holds.
func (s *Service) routerProvidersFor(o appOptions, existing []models.ModelRouter) ([]privacyResource, error) {
	var ids []uint
	if o.modelRouterIDs != nil {
		ids = *o.modelRouterIDs
	} else {
		for _, r := range existing {
			ids = append(ids, r.ID)
		}
	}
	return s.modelRouterProviders(ids)
}

// setAppModelRouters replaces the App's router grants.
func (s *Service) setAppModelRouters(app *models.App, ids []uint) error {
	var routers []models.ModelRouter
	if len(ids) > 0 {
		if err := s.DB.Where("id IN ?", ids).Find(&routers).Error; err != nil {
			return err
		}
	}
	if err := s.DB.Model(app).Association("ModelRouters").Replace(routers); err != nil {
		return fmt.Errorf("failed to set app model routers: %w", err)
	}
	return nil
}

// ClearAppModelRouters removes every router grant (App deletion).
func (s *Service) ClearAppModelRouters(appID uint) error {
	app := models.App{}
	app.ID = appID
	return s.DB.Model(&app).Association("ModelRouters").Clear()
}

// SetModelRouterCatalogues replaces the LLM catalogues a router is published
// in. Every catalogue must exist.
func (s *Service) SetModelRouterCatalogues(routerID uint, catalogueIDs []uint) ([]models.Catalogue, error) {
	var router models.ModelRouter
	if err := s.DB.First(&router, routerID).Error; err != nil {
		return nil, err
	}
	ids := uniqueUintIDs(catalogueIDs)
	var catalogues []models.Catalogue
	if len(ids) > 0 {
		if err := s.DB.Where("id IN ?", ids).Find(&catalogues).Error; err != nil {
			return nil, err
		}
		if len(catalogues) != len(ids) {
			return nil, fmt.Errorf("%w: one or more catalogues do not exist", ErrInvalidCatalogueReference)
		}
	}
	if err := s.DB.Model(&router).Association("Catalogues").Replace(catalogues); err != nil {
		return nil, fmt.Errorf("failed to set model router catalogues: %w", err)
	}
	return catalogues, nil
}

// GetModelRouterCatalogues lists the LLM catalogues a router is published in.
func (s *Service) GetModelRouterCatalogues(routerID uint) ([]models.Catalogue, error) {
	var router models.ModelRouter
	if err := s.DB.Preload("Catalogues").First(&router, routerID).Error; err != nil {
		return nil, err
	}
	return router.Catalogues, nil
}

// ErrInvalidCatalogueReference is returned when a catalogue id does not exist.
var ErrInvalidCatalogueReference = errors.New("invalid catalogue reference")
