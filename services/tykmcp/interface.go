// Package tykmcp defines the Tyk Dashboard MCP integration: MCP proxies
// defined in a Tyk Dashboard are discovered and published to the AI Portal as
// an asset class, registered from Studio into the Dashboard, proposed by
// portal users through the submission workflow, and access keys are brokered
// against Tyk security policies.
//
// The Tyk Gateway is the only MCP data plane; Studio is the catalogue, the
// approval workflow, the credential broker and the audit ledger.
//
// Community Edition ships a no-op implementation that answers every query
// with ErrEnterpriseFeature. Enterprise Edition registers the real one via
// the factory in this package.
package tykmcp

import (
	"context"
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ErrEnterpriseFeature is returned by Community Edition for every operation.
var ErrEnterpriseFeature = errors.New("the Tyk Dashboard MCP integration is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// Errors shared by both editions. Handlers map them onto HTTP statuses.
var (
	ErrDisabled              = errors.New("the Tyk Dashboard MCP integration is disabled")
	ErrNotFound              = errors.New("tyk mcp resource not found")
	ErrConflict              = errors.New("the record was modified by someone else; reload and retry")
	ErrInvalidState          = errors.New("operation not allowed in the current state")
	ErrURLPolicy             = errors.New("url is not allowed by the Tyk Dashboard URL policy")
	ErrValidation            = errors.New("invalid input")
	ErrCapabilityUnavailable = errors.New("the Tyk Dashboard connection does not permit this operation")
	ErrSameActivator         = errors.New("a connection must be activated by a different administrator than the one who created it")
	ErrDashboard             = errors.New("tyk dashboard request failed")
	ErrDashboardConflict     = errors.New("the definition changed on the Tyk Dashboard since it was loaded; reload and retry")
	ErrInvalidBundle         = errors.New("the policy bundle is invalid")
	ErrNotBrokerable         = errors.New("no access key can be minted for this MCP server")
	ErrNotVisible            = errors.New("MCP server is not available to you")
	ErrInUse                 = errors.New("the connection still has live credentials")
)

// Actor is the administrator or portal user performing an operation.
type Actor struct {
	UserID uint
	Email  string
	Name   string
	// CanExecute reports whether the actor holds the execute permission that
	// lets widening credential changes apply without a review.
	CanExecute bool
}

// ConnectionInput creates a connection.
type ConnectionInput struct {
	Name                  string                 `json:"name"`
	Description           string                 `json:"description"`
	DashboardURL          string                 `json:"dashboard_url"`
	GatewayBaseURL        string                 `json:"gateway_base_url"`
	DashboardAccessToken  string                 `json:"dashboard_access_token"`
	OrgID                 string                 `json:"org_id"`
	DeclaredMode          string                 `json:"declared_mode"`
	SyncIntervalSeconds   int                    `json:"sync_interval_seconds"`
	AutoPublish           bool                   `json:"auto_publish"`
	DefaultPrivacyScore   *int                   `json:"default_privacy_score"`
	AcceptHandoffs        *bool                  `json:"accept_handoffs"`
	KeyDefaults           *models.TykKeyDefaults `json:"key_defaults"`
	AllowInternalHost     bool                   `json:"allow_internal_host"`
	MDCBURL               string                 `json:"mdcb_url"`
	MDCBAccessToken       string                 `json:"mdcb_access_token"`
	MDCBAllowInternalHost bool                   `json:"mdcb_allow_internal_host"`
	KnownGatewayTags      []models.TykGatewayTag `json:"known_gateway_tags"`
	GatewayBaseURLs       map[string]string      `json:"gateway_base_urls"`
}

// ConnectionPatch updates a connection. Nil fields are left unchanged. An
// empty token string leaves the stored token alone; a new value replaces it.
type ConnectionPatch struct {
	Name                  *string                 `json:"name"`
	Description           *string                 `json:"description"`
	DashboardURL          *string                 `json:"dashboard_url"`
	GatewayBaseURL        *string                 `json:"gateway_base_url"`
	DashboardAccessToken  *string                 `json:"dashboard_access_token"`
	OrgID                 *string                 `json:"org_id"`
	DeclaredMode          *string                 `json:"declared_mode"`
	SyncIntervalSeconds   *int                    `json:"sync_interval_seconds"`
	AutoPublish           *bool                   `json:"auto_publish"`
	DefaultPrivacyScore   *int                    `json:"default_privacy_score"`
	ClearPrivacyScore     bool                    `json:"clear_default_privacy_score"`
	AcceptHandoffs        *bool                   `json:"accept_handoffs"`
	KeyDefaults           *models.TykKeyDefaults  `json:"key_defaults"`
	AllowInternalHost     *bool                   `json:"allow_internal_host"`
	MDCBURL               *string                 `json:"mdcb_url"`
	MDCBAccessToken       *string                 `json:"mdcb_access_token"`
	MDCBAllowInternalHost *bool                   `json:"mdcb_allow_internal_host"`
	KnownGatewayTags      *[]models.TykGatewayTag `json:"known_gateway_tags"`
	GatewayBaseURLs       *map[string]string      `json:"gateway_base_urls"`
	LockVersion           int                     `json:"lock_version"`
}

// ProbeResult is what the capability probe learned about a Dashboard.
type ProbeResult struct {
	Capabilities  map[string]models.TykCapability `json:"capabilities"`
	EffectiveMode string                          `json:"effective_mode"`
	DeclaredMode  string                          `json:"declared_mode"`
	OrgID         string                          `json:"org_id,omitempty"`
	DataPlanes    []models.TykDataPlane           `json:"data_planes"`
	Warnings      []string                        `json:"warnings"`
	Reachable     bool                            `json:"reachable"`
	ProbedAt      time.Time                       `json:"probed_at"`
}

// Status describes the running feature so the UI can explain itself.
type Status struct {
	Available bool `json:"available"`
	Enabled   bool `json:"enabled"`
	// DisabledReason explains why Enabled is false when the operator did not
	// switch the feature off (e.g. the secrets encryption key is missing).
	DisabledReason      string `json:"disabled_reason,omitempty"`
	NodeID              string `json:"node_id,omitempty"`
	Connections         int64  `json:"connections"`
	ActiveConnections   int64  `json:"active_connections"`
	DegradedConnections int64  `json:"degraded_connections"`
}

// ServerFilter narrows ListServers. Zero values mean "no filter".
type ServerFilter struct {
	ConnectionID uint
	State        string // dashboard_state
	Origin       string
	Kind         string
	Published    *bool
	Search       string // case-insensitive substring over name, slug, description, listen path
	Page         int    // 1-based
	PageSize     int
}

// ServerList is one page of servers.
type ServerList struct {
	Servers  []models.MCPServerResponse `json:"servers"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

// ServerPatch edits the Studio-owned presentation and governance fields of
// a server. Definition-derived fields cannot be edited here.
type ServerPatch struct {
	Name              *string   `json:"name"`
	NameOverridden    *bool     `json:"name_overridden"`
	Description       *string   `json:"description"`
	LongDescription   *string   `json:"long_description"`
	LogoURL           *string   `json:"logo_url"`
	Tags              *[]string `json:"tags"`
	PrivacyScore      *int      `json:"privacy_score"`
	ClearPrivacyScore bool      `json:"clear_privacy_score"`
	LockVersion       int       `json:"lock_version"`
}

// PinInput names one policy in a bundle by its Tyk policy id.
type PinInput struct {
	TykPolicyID string `json:"tyk_policy_id"`
	Role        string `json:"role"`
}

// PolicyFilter narrows ListPolicies.
type PolicyFilter struct {
	MCPOnly bool   // only policies whose access rights name an MCP proxy
	APIID   string // only policies granting this api id
	Search  string
}

// Paging bounds.
const (
	DefaultPageSize = 25
	MaxPageSize     = 200
)

// Service is the Tyk Dashboard MCP integration contract shared by both editions.
type Service interface {
	// MCP servers (discovery)
	ListServers(ctx context.Context, f ServerFilter) (*ServerList, error)
	GetServer(ctx context.Context, id uint) (*models.MCPServerResponse, error)
	UpdateServer(ctx context.Context, actor Actor, id uint, p ServerPatch) (*models.MCPServerResponse, error)
	// DeleteServer removes a record the Dashboard no longer has (missing) or
	// that never reached it (pending_platform). Studio-origin proxies are
	// deleted on the Dashboard by the registration milestone.
	DeleteServer(ctx context.Context, actor Actor, id uint) error
	PublishServer(ctx context.Context, actor Actor, id uint) (*models.MCPServerResponse, error)
	UnpublishServer(ctx context.Context, actor Actor, id uint) (*models.MCPServerResponse, error)
	SetServerGroups(ctx context.Context, actor Actor, id uint, groupIDs []uint) (*models.MCPServerResponse, error)
	SetServerBundle(ctx context.Context, actor Actor, id uint, pins []PinInput) (*models.MCPServerResponse, error)

	// Policies and sync
	ListPolicies(ctx context.Context, connectionID uint, f PolicyFilter) ([]models.TykPolicyResponse, error)
	ListSyncRuns(ctx context.Context, connectionID uint, limit int) ([]models.MCPSyncRun, error)
	// RunSync performs a discovery sync of one connection on this node and
	// returns the run record.
	RunSync(ctx context.Context, actor Actor, connectionID uint) (*models.MCPSyncRun, error)

	// Connections
	ListConnections(ctx context.Context) ([]models.TykConnectionResponse, error)
	GetConnection(ctx context.Context, id uint) (*models.TykConnectionResponse, error)
	CreateConnection(ctx context.Context, actor Actor, in ConnectionInput) (*models.TykConnectionResponse, error)
	UpdateConnection(ctx context.Context, actor Actor, id uint, p ConnectionPatch) (*models.TykConnectionResponse, error)
	// DeleteConnection refuses while live credentials exist unless force is set,
	// in which case every key is revoked first.
	DeleteConnection(ctx context.Context, actor Actor, id uint, force bool) error
	// ActivateConnection probes the Dashboard and, when it is reachable, makes
	// the connection usable.
	ActivateConnection(ctx context.Context, actor Actor, id uint) (*models.TykConnectionResponse, error)
	DisableConnection(ctx context.Context, actor Actor, id uint, reason string) (*models.TykConnectionResponse, error)
	// ProbeConnection re-runs the capability probe on a stored connection.
	ProbeConnection(ctx context.Context, actor Actor, id uint) (*ProbeResult, error)
	// ProbeInput runs the capability probe against unsaved form values so an
	// administrator can test before saving. Nothing is persisted.
	ProbeInput(ctx context.Context, in ConnectionInput) (*ProbeResult, error)
	// TriggerSync asks for the next sync to run as soon as a node polls.
	TriggerSync(ctx context.Context, actor Actor, id uint) error

	// Lifecycle
	Status() Status
	Stop()
}
