// Package workspaces manages Workspaces: the ownership boundary of objects
// (models.Workspace, models.WorkspaceStampedModels). Community Edition
// ships a stub in which only the Default workspace exists and every
// management call answers ErrEnterpriseFeature; Enterprise Edition
// registers the real implementation through the factory.
package workspaces

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// Errors shared by both editions.
var (
	ErrEnterpriseFeature  = errors.New("Workspaces are an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")
	ErrNotFound           = errors.New("workspace not found")
	ErrInvalidName        = errors.New("a Workspace needs a name of at most 128 characters")
	ErrInvalidDescription = errors.New("a Workspace description has at most 1024 characters")
	ErrDuplicateName      = errors.New("a Workspace with that name already exists")
	ErrDefaultLocked      = errors.New("the Default workspace cannot be deleted or changed into another Workspace")
	ErrInUse              = errors.New("roles are still assigned within this Workspace; remove those assignments first")
)

// Input is the payload for creating or updating a Workspace. A nil field is
// left unchanged on update.
type Input struct {
	Name        *string
	Description *string
}

// Resource names one kind of object a Workspace owns: the permission
// resource key and the model whose table carries workspace_id.
type Resource struct {
	Key   string
	Model interface{}
}

// CountedResources are the resource kinds GET /workspaces/:id/objects
// reports, the Workspace resources whose objects carry their own stamp.
// Plugins and webhook targets are stamped too but are global in this
// release (.36), so their stamp is neither enforced nor shown.
func CountedResources() []Resource {
	return []Resource{
		{"llms", &models.LLM{}},
		{"tools", &models.Tool{}},
		{"datasources", &models.Datasource{}},
		{"filters", &models.Filter{}},
		{"embedders", &models.Embedder{}},
		{"secrets", &secrets.Secret{}},
		{"filestores", &models.FileStore{}},
		{"mcp-servers", &models.MCPServer{}},
		{"apps", &models.App{}},
		{"agents", &models.AgentConfig{}},
		{"model-routers", &models.ModelRouter{}},
		{"semantic-routers", &models.SemanticRouter{}},
		{"catalogues", &models.Catalogue{}},
		{"data-catalogues", &models.DataCatalogue{}},
		{"tool-catalogues", &models.ToolCatalogue{}},
	}
}

// Service is the Workspaces contract shared by both editions.
type Service interface {
	// Enabled reports whether Workspaces can be managed: an Enterprise build
	// with role-based access control licensed.
	Enabled() bool

	// List returns Workspaces, the Default first, then by name: every
	// Workspace when only is nil, otherwise those whose id is in only (none
	// for an empty, non-nil slice).
	List(ctx context.Context, only []uint) ([]models.Workspace, error)
	// Get returns one Workspace (ErrNotFound).
	Get(ctx context.Context, id uint) (*models.Workspace, error)
	// Create adds a Workspace. Names are unique without regard to case.
	Create(ctx context.Context, actor *models.User, in Input) (*models.Workspace, error)
	// Update renames or re-describes a Workspace. is_default never changes.
	Update(ctx context.Context, actor *models.User, id uint, in Input) (*models.Workspace, error)
	// Delete removes a Workspace that no role binding is scoped to, moving
	// every object it owns to the Default workspace (workspace_id and
	// updated_at written together, so the moves show as pending changes).
	// ErrDefaultLocked for the Default, ErrInUse while a binding is scoped
	// to it. It runs in one transaction that locks the Workspace row, so a
	// binding or object created concurrently on any replica either lands
	// before the check (and the delete is refused) or is refused itself.
	Delete(ctx context.Context, actor *models.User, id uint) error
	// Counts returns, per CountedResources key, how many objects the
	// Workspace owns (soft-deleted ones excluded). Rows still carrying
	// workspace_id 0 count for the Default workspace.
	Counts(ctx context.Context, id uint) (map[string]int64, error)
}
