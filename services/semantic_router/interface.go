// Package semantic_router manages Semantic Routers (Enterprise): routers that
// pick one of their named routes from what the prompt says. The engine that
// classifies lives in the Enterprise module (see pkg/semanticrouting); this
// package is the service boundary the API and the snapshot use.
package semantic_router

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"gorm.io/gorm"
)

var (
	// ErrEnterpriseFeature is returned by every method in the Community Edition.
	ErrEnterpriseFeature = errors.New("semantic router is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

	// ErrNotFound is returned when the router does not exist.
	ErrNotFound = errors.New("semantic router not found")

	// ErrInvalid wraps a configuration the service refuses (a bad slug, a
	// route that targets an LLM that does not exist, ...). Structural
	// problems come back as *semanticrouting.ValidationError instead.
	ErrInvalid = errors.New("invalid semantic router")
)

// Service manages Semantic Routers.
type Service interface {
	CreateRouter(router *models.SemanticRouter) error
	GetRouter(id uint) (*models.SemanticRouter, error)
	// UpdateRouter saves the router's configuration and presentation; its
	// catalogues are set separately.
	UpdateRouter(router *models.SemanticRouter) error
	DeleteRouter(id uint) error
	// ListRouters lists routers, paged unless all is set; scopes filter and
	// sort (see services.ListOptions).
	ListRouters(pageSize, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) ([]models.SemanticRouter, int64, int, error)
	ToggleRouterActive(id uint, active bool) error
	// ValidateRouter checks the configuration and everything it references.
	ValidateRouter(router *models.SemanticRouter) error
	// Test classifies messages with the router's configuration, as the edge
	// would, and returns the decision with its per-stage trace. The router
	// need not be saved or active. Examples are embedded first.
	Test(ctx context.Context, router *models.SemanticRouter, req sr.Request) (*sr.Decision, error)
}
