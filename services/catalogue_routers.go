package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// Routers seen from an LLM catalogue. A router's catalogue memberships can be
// set from the router (SetModelRouterCatalogues, SetSemanticRouterCatalogues)
// or from the catalogue (SetCatalogueRouters); both edit the same join tables.

// ErrInvalidRouterReference is returned when a router id does not exist.
var ErrInvalidRouterReference = errors.New("invalid router reference")

// CatalogueRouters are the routers published in one LLM catalogue.
type CatalogueRouters struct {
	ModelRouters    []models.ModelRouter
	SemanticRouters []models.SemanticRouter
}

// GetCatalogueRouters lists the routers published in the catalogue.
func (s *Service) GetCatalogueRouters(catalogueID uint) (*CatalogueRouters, error) {
	var cat models.Catalogue
	if err := s.DB.Preload("ModelRouters", func(db *gorm.DB) *gorm.DB { return db.Order("model_routers.name") }).
		Preload("SemanticRouters", func(db *gorm.DB) *gorm.DB { return db.Order("semantic_routers.name") }).
		First(&cat, catalogueID).Error; err != nil {
		return nil, err
	}
	return &CatalogueRouters{ModelRouters: cat.ModelRouters, SemanticRouters: cat.SemanticRouters}, nil
}

// SetCatalogueRouters replaces the catalogue's routers of each kind given; a
// nil list leaves that kind as it is. Every router must exist.
func (s *Service) SetCatalogueRouters(catalogueID uint, modelRouterIDs, semanticRouterIDs *[]uint) (*CatalogueRouters, error) {
	var cat models.Catalogue
	if err := s.DB.First(&cat, catalogueID).Error; err != nil {
		return nil, err
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if modelRouterIDs != nil {
			ids := uniqueUintIDs(*modelRouterIDs)
			var routers []models.ModelRouter
			if len(ids) > 0 {
				if err := tx.Where("id IN ?", ids).Find(&routers).Error; err != nil {
					return err
				}
				if len(routers) != len(ids) {
					return fmt.Errorf("%w: one or more model routers do not exist", ErrInvalidRouterReference)
				}
			}
			if err := tx.Model(&cat).Association("ModelRouters").Replace(routers); err != nil {
				return err
			}
		}
		if semanticRouterIDs != nil {
			ids := uniqueUintIDs(*semanticRouterIDs)
			var routers []models.SemanticRouter
			if len(ids) > 0 {
				if err := tx.Where("id IN ?", ids).Find(&routers).Error; err != nil {
					return err
				}
				if len(routers) != len(ids) {
					return fmt.Errorf("%w: one or more semantic routers do not exist", ErrInvalidRouterReference)
				}
			}
			if err := tx.Model(&cat).Association("SemanticRouters").Replace(routers); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetCatalogueRouters(catalogueID)
}
