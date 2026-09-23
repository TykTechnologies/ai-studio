package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Semantic Routers as App assets, granted and published exactly like Model
// Routers (see app_model_routers.go). For the privacy rule a Semantic Router
// is scored by the least private LLM it may send a request's text to: the
// LLMs its routes target, the vendors of the Model Routers it hands off to,
// and its embedding and judge LLMs (models.SemanticRouterPrivacySQL).

// ErrSemanticRouterNotVisible is returned when an App would be granted a
// Semantic Router the caller's teams cannot see, or one that is not active.
var ErrSemanticRouterNotVisible = errors.New("semantic router is not available to this app")

// WithSemanticRouters sets the App's Semantic Router grants. Callers check
// visibility first (ValidateSemanticRouterBindings).
func WithSemanticRouters(ids []uint) AppOption {
	return func(o *appOptions) {
		cp := uniqueUintIDs(ids)
		o.semanticRouterIDs = &cp
	}
}

// ValidateSemanticRouterBindings checks that every router may be granted by
// this user: active, and in an LLM catalogue one of the user's teams holds (an
// administrator may grant any active router).
func (s *Service) ValidateSemanticRouterBindings(userID uint, isAdmin bool, routerIDs []uint) ([]models.SemanticRouter, error) {
	ids := uniqueUintIDs(routerIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	q := s.DB.Model(&models.SemanticRouter{})
	if isAdmin {
		q = q.Where("semantic_routers.active = ?", true)
	} else {
		q = models.AccessibleSemanticRouterQuery(s.DB, userID)
	}
	var routers []models.SemanticRouter
	if err := q.Where("semantic_routers.id IN ?", ids).Group("semantic_routers.id").Find(&routers).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]models.SemanticRouter, len(routers))
	for _, r := range routers {
		byID[r.ID] = r
	}
	out := make([]models.SemanticRouter, 0, len(ids))
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: router %d", ErrSemanticRouterNotVisible, id)
		}
		out = append(out, r)
	}
	return out, nil
}

// SemanticRouterReachableLLMs lists the active LLMs a router's routes can
// answer with: what a grant of the router lets an App reach.
func (s *Service) SemanticRouterReachableLLMs(routerID uint) ([]models.LLM, error) {
	return models.SemanticRouterReachableLLMs(s.DB, routerID)
}

// semanticRouterProviders scores routers as providers for the privacy rule.
// A router that reaches no active LLM is left out.
func (s *Service) semanticRouterProviders(routerIDs []uint) ([]privacyResource, error) {
	ids := uniqueUintIDs(routerIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	var routers []models.SemanticRouter
	if err := s.DB.Select("id", "name").Where("id IN ?", ids).Find(&routers).Error; err != nil {
		return nil, err
	}
	if len(routers) != len(ids) {
		return nil, fmt.Errorf("%w: one or more semantic routers do not exist", ErrSemanticRouterNotVisible)
	}
	scores, err := models.SemanticRouterPrivacyScores(s.DB, ids)
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

// setAppSemanticRouters replaces the App's Semantic Router grants.
func (s *Service) setAppSemanticRouters(app *models.App, ids []uint) error {
	var routers []models.SemanticRouter
	if len(ids) > 0 {
		if err := s.DB.Where("id IN ?", ids).Find(&routers).Error; err != nil {
			return err
		}
	}
	if err := s.DB.Model(app).Association("SemanticRouters").Replace(routers); err != nil {
		return fmt.Errorf("failed to set app semantic routers: %w", err)
	}
	return nil
}

// ClearAppSemanticRouters removes every Semantic Router grant (App deletion).
func (s *Service) ClearAppSemanticRouters(appID uint) error {
	app := models.App{}
	app.ID = appID
	return s.DB.Model(&app).Association("SemanticRouters").Clear()
}

// SetSemanticRouterCatalogues replaces the LLM catalogues a router is
// published in. Every catalogue must exist.
func (s *Service) SetSemanticRouterCatalogues(routerID uint, catalogueIDs []uint) ([]models.Catalogue, error) {
	var router models.SemanticRouter
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
		return nil, fmt.Errorf("failed to set semantic router catalogues: %w", err)
	}
	return catalogues, nil
}

// GetSemanticRouterCatalogues lists the LLM catalogues a router is published in.
func (s *Service) GetSemanticRouterCatalogues(routerID uint) ([]models.Catalogue, error) {
	var router models.SemanticRouter
	if err := s.DB.Preload("Catalogues").First(&router, routerID).Error; err != nil {
		return nil, err
	}
	return router.Catalogues, nil
}
