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
	"encoding/json"
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
	TemplateID            string                 `json:"template_id"`
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
	TemplateID            *string                 `json:"template_id"`
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

// AppMCPServerView is one MCP server as seen from an App.
type AppMCPServerView struct {
	ID             uint                                 `json:"id"`
	ConnectionID   *uint                                `json:"connection_id"`
	ConnectionName string                               `json:"connection_name,omitempty"`
	Name           string                               `json:"name"`
	Slug           string                               `json:"slug"`
	Kind           string                               `json:"kind"`
	AuthMode       string                               `json:"auth_mode"`
	EndpointURL    string                               `json:"endpoint_url"`
	EndpointURLs   map[string]string                    `json:"endpoint_urls"`
	PRM            *models.MCPProtectedResourceMetadata `json:"prm,omitempty"`
	HeaderName     string                               `json:"header_name,omitempty"`
	Brokerable     bool                                 `json:"brokerable"`
	GrantKind      string                               `json:"grant_kind"`
	GrantOpen      bool                                 `json:"grant_open"`
}

// AppMCPSummary is the App page's MCP section: the servers the App reaches
// and the credentials minted for it, one per connection.
type AppMCPSummary struct {
	Servers     []AppMCPServerView             `json:"servers"`
	Credentials []models.MCPCredentialResponse `json:"credentials"`
	// Connections lists, per connection the App touches, whether a key can
	// be minted now and why not otherwise.
	Connections []AppMCPConnectionState `json:"connections"`
}

// AppMCPConnectionState summarises one connection for the App page.
type AppMCPConnectionState struct {
	ConnectionID   uint   `json:"connection_id"`
	ConnectionName string `json:"connection_name"`
	CanMint        bool   `json:"can_mint"`
	Reason         string `json:"reason,omitempty"`
	CredentialID   string `json:"credential_id,omitempty"`
}

// MintInput names the App and connection to mint for.
type MintInput struct {
	AppID        uint `json:"app_id"`
	ConnectionID uint `json:"connection_id"`
}

// SkippedServer is a bound server the key could not cover.
type SkippedServer struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// MintedCredential is the mint result. Key is returned exactly once.
type MintedCredential struct {
	Credential models.MCPCredentialResponse `json:"credential"`
	Key        string                       `json:"key"`
	Servers    []AppMCPServerView           `json:"servers"`
	Skipped    []SkippedServer              `json:"skipped"`
}

// CredentialFilter narrows ListCredentials.
type CredentialFilter struct {
	AppID        uint
	ConnectionID uint
	ServerID     uint // credentials of Apps bound to this server
	UserID       uint // credentials of Apps owned by this user
	Status       string
	Drift        string
	Page         int
	PageSize     int
}

// CredentialList is one page of credentials.
type CredentialList struct {
	Credentials []models.MCPCredentialResponse `json:"credentials"`
	Total       int64                          `json:"total"`
	Page        int                            `json:"page"`
	PageSize    int                            `json:"page_size"`
}

// AccessReportRow answers "who has access to what": one row per open (or,
// when IncludeRevoked, any) grant with the credential that backs it.
type AccessReportRow struct {
	GrantID          uint       `json:"grant_id"`
	AppID            uint       `json:"app_id"`
	AppName          string     `json:"app_name"`
	UserID           uint       `json:"user_id"`
	UserEmail        string     `json:"user_email"`
	ServerID         uint       `json:"server_id"`
	ServerName       string     `json:"server_name"`
	ConnectionID     *uint      `json:"connection_id"`
	ConnectionName   string     `json:"connection_name,omitempty"`
	GrantKind        string     `json:"grant_kind"`
	GrantedAt        time.Time  `json:"granted_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokeReason     string     `json:"revoke_reason,omitempty"`
	CredentialID     string     `json:"credential_id,omitempty"`
	CredentialHash   string     `json:"credential_hash,omitempty"`
	CredentialStatus string     `json:"credential_status,omitempty"`
}

// ReportFilter narrows AccessReport.
type ReportFilter struct {
	ConnectionID   uint
	ServerID       uint
	UserID         uint
	AppID          uint
	IncludeRevoked bool
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

// --- Registration (full mode) ---

// Consumer authentication shapes the registration wizard can produce.
const (
	ConsumerAuthToken   = "auth_token"
	ConsumerAuthOAuth21 = "oauth21"
	ConsumerAuthKeyless = "keyless"
)

// RegisterPrimitive is one REST operation exposed as an MCP tool by a
// REST-to-MCP proxy. Either OperationID or Method+Path names the source.
type RegisterPrimitive struct {
	OperationID string                 `json:"operation_id"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Annotations map[string]interface{} `json:"annotations"`
}

// RegisterInput describes an MCP proxy to create on a Tyk Dashboard.
// UpstreamAuthToken is sent to the Dashboard and never persisted in Studio.
type RegisterInput struct {
	ConnectionID    uint   `json:"connection_id"`
	Kind            string `json:"kind"` // remote | rest_to_mcp
	Name            string `json:"name"`
	ListenPath      string `json:"listen_path"`
	StripListenPath *bool  `json:"strip_listen_path"` // default true

	// Remote MCP server
	UpstreamURL            string   `json:"upstream_url"`
	UpstreamAuthHeaderName string   `json:"upstream_auth_header_name"`
	UpstreamAuthToken      string   `json:"upstream_auth_token"`
	AllowedTools           []string `json:"allowed_tools"`

	// REST API to MCP
	SourceAPIID string              `json:"source_api_id"`
	Primitives  []RegisterPrimitive `json:"primitives"`

	// Consumer authentication
	ConsumerAuth         string   `json:"consumer_auth"` // auth_token | oauth21 | keyless
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
	ConfirmKeyless       bool     `json:"confirm_keyless"`

	// Deployment target (segmented gateways)
	GatewayTags          []string `json:"gateway_tags"`
	ConfirmNoGatewayTags bool     `json:"confirm_no_gateway_tags"`

	// Studio presentation and governance
	Description      string   `json:"description"`
	LongDescription  string   `json:"long_description"`
	Tags             []string `json:"tags"`
	PrivacyScore     *int     `json:"privacy_score"`
	Publish          bool     `json:"publish"`
	ToolCatalogueIDs []uint   `json:"tool_catalogue_ids"`
}

// RegisterPreview is a dry-run result: the definition Studio would send
// (secrets masked), the Dashboard's expanded rendering when it returned one,
// and warnings the administrator should read before confirming.
type RegisterPreview struct {
	Definition  json.RawMessage `json:"definition"`
	Rendered    json.RawMessage `json:"rendered,omitempty"`
	Warnings    []string        `json:"warnings"`
	EndpointURL string          `json:"endpoint_url,omitempty"`
	// DashboardValidated is always false today: Dashboard 5.14 persists dry
	// runs instead of validating them, so previews are Studio's own render
	// and checks and the Dashboard validates on create or push.
	DashboardValidated bool `json:"dashboard_validated"`
}

// PushInput replaces a server's definition on the Dashboard. Definition is
// the edited document; masked values ("***") are restored from the live
// document before sending. ExpectedHash is the hash the editor loaded; a
// different live hash is a conflict.
type PushInput struct {
	Definition             json.RawMessage `json:"definition"`
	ExpectedHash           string          `json:"expected_hash"`
	ConfirmDashboardOrigin bool            `json:"confirm_dashboard_origin"`
}

// SourceAPI is a Tyk OAS API that can back a REST-to-MCP proxy.
type SourceAPI struct {
	APIID      string `json:"api_id"`
	Name       string `json:"name"`
	ListenPath string `json:"listen_path"`
	Active     bool   `json:"active"`
}

// SourceOperation is one operation of a source API.
type SourceOperation struct {
	OperationID string `json:"operation_id"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
}

// SourceAPIDocument is a Tyk OAS API's definition as the Tools import
// stores it: the OAS document with the vendor extension kept and every
// credential under x-tyk-api-gateway masked.
type SourceAPIDocument struct {
	APIID      string          `json:"api_id"`
	Name       string          `json:"name"`
	ListenPath string          `json:"listen_path"`
	Active     bool            `json:"active"`
	Definition json.RawMessage `json:"definition"`
}

// GatewayTagOption is one deployment target the wizard can offer.
type GatewayTagOption struct {
	Tag         string                `json:"tag"`
	Label       string                `json:"label,omitempty"`
	Description string                `json:"description,omitempty"`
	Sources     []string              `json:"sources"` // mdcb | known | proxies
	DataPlanes  []models.TykDataPlane `json:"data_planes"`
	// Verified is true when MDCB currently reports a data plane with this tag.
	Verified bool `json:"verified"`
}

// PolicyInput is the minimal policy creator's input. An access policy
// grants one MCP server; a consumption policy carries rate and quota limits.
type PolicyInput struct {
	Kind             string `json:"kind"` // access | consumption
	Name             string `json:"name"`
	ServerID         uint   `json:"server_id"`
	Rate             int64  `json:"rate"`
	Per              int64  `json:"per"`
	QuotaMax         int64  `json:"quota_max"`
	QuotaRenewalRate int64  `json:"quota_renewal_rate"`
	KeyExpiresIn     int64  `json:"key_expires_in"`
	// Pin adds the new policy to ServerID's bundle straight away.
	Pin bool `json:"pin"`
}

const (
	PolicyKindAccess      = "access"
	PolicyKindConsumption = "consumption"
)

// Service is the Tyk Dashboard MCP integration contract shared by both editions.
type Service interface {
	// MCP servers (discovery)
	ListServers(ctx context.Context, f ServerFilter) (*ServerList, error)
	GetServer(ctx context.Context, id uint) (*models.MCPServerResponse, error)
	UpdateServer(ctx context.Context, actor Actor, id uint, p ServerPatch) (*models.MCPServerResponse, error)
	// DeleteServer removes a record the Dashboard no longer has (missing) or
	// that never reached it (pending_platform), or, on a full connection,
	// deletes a Studio-registered proxy from the Dashboard. force revokes the
	// keys that would otherwise block the delete.
	DeleteServer(ctx context.Context, actor Actor, id uint, force bool) error

	// Registration (full mode)
	// RegisterServer creates an MCP proxy on the Dashboard from the wizard's
	// input. With dryRun the Dashboard only validates and the preview is
	// returned; otherwise the catalogued server is returned.
	RegisterServer(ctx context.Context, actor Actor, in RegisterInput, dryRun bool) (*RegisterPreview, *models.MCPServerResponse, error)
	// PushServer replaces a server's definition on the Dashboard after
	// checking the live hash and restoring masked secrets.
	PushServer(ctx context.Context, actor Actor, id uint, in PushInput, dryRun bool) (*RegisterPreview, *models.MCPServerResponse, error)
	ListSourceAPIs(ctx context.Context, connectionID uint, search string) ([]SourceAPI, error)
	ListSourceOperations(ctx context.Context, connectionID uint, apiID string) ([]SourceOperation, error)
	// GetSourceAPIDocument fetches one Tyk OAS API for the Tools import,
	// masking upstream credentials before it leaves the service.
	GetSourceAPIDocument(ctx context.Context, connectionID uint, apiID string) (*SourceAPIDocument, error)
	// GatewayTagOptions lists the deployment targets known for a connection:
	// MDCB-discovered, administrator-known and seen on synced proxies. Empty
	// means the control should not render.
	GatewayTagOptions(ctx context.Context, connectionID uint) ([]GatewayTagOption, error)
	// CreatePolicy creates a partitioned Studio-managed policy on the
	// Dashboard, caches it and optionally pins it.
	CreatePolicy(ctx context.Context, actor Actor, connectionID uint, in PolicyInput) (*models.TykPolicyResponse, error)
	// UpdatePolicy edits a Studio-managed policy (name and limits only).
	UpdatePolicy(ctx context.Context, actor Actor, connectionID uint, tykPolicyID string, in PolicyInput) (*models.TykPolicyResponse, error)

	// Community registration
	// SubmissionConnections lists the connections a portal user may submit an
	// MCP server to, with what each one knows about deployment targets.
	SubmissionConnections(ctx context.Context) ([]SubmissionConnection, error)
	// ValidateSubmissionInput checks a submission payload against its
	// connection without contacting the Dashboard.
	ValidateSubmissionInput(ctx context.Context, in RegisterInput) error
	// RegisterFromSubmission is phase one of an approval: on a full-mode
	// connection it creates the proxy (idempotently: an existing catalogue
	// row for the submission, or a proxy on the same listen path, is adopted);
	// otherwise it records a pending_platform server and emits the handoff.
	RegisterFromSubmission(ctx context.Context, actor Actor, reg SubmissionRegistration) (*models.MCPServerResponse, error)
	// HandoffPackage renders what the platform team needs for a
	// pending_platform server. Secrets are included only for an actor who may
	// execute, and the download is audited.
	HandoffPackage(ctx context.Context, actor Actor, serverID uint, includeSecrets bool) (*HandoffPackage, error)
	// LinkServer joins a pending_platform server to the proxy the platform
	// team created (imported by sync under tykAPIID).
	LinkServer(ctx context.Context, actor Actor, pendingID uint, tykAPIID string) (*models.MCPServerResponse, error)
	PublishServer(ctx context.Context, actor Actor, id uint) (*models.MCPServerResponse, error)
	UnpublishServer(ctx context.Context, actor Actor, id uint) (*models.MCPServerResponse, error)
	// SetServerCatalogues replaces the tool catalogues the server belongs to;
	// teams see it through the catalogues they are granted.
	SetServerCatalogues(ctx context.Context, actor Actor, id uint, catalogueIDs []uint) (*models.MCPServerResponse, error)
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

	// Apps
	// SyncAppGrants reconciles the access-grant ledger of an App with its
	// bound MCP servers and its credential state. Called by the App service
	// after binding changes, activation, deactivation and deletion.
	SyncAppGrants(ctx context.Context, appID uint) error
	// AppMCPSummary describes, for the portal App page, the servers an App
	// reaches and (in later milestones) its minted credentials.
	AppMCPSummary(ctx context.Context, appID uint) (*AppMCPSummary, error)

	// Credentials (broker)
	// MintCredential mints a Tyk key for an App on one connection and
	// returns the plaintext exactly once. The caller has checked ownership.
	MintCredential(ctx context.Context, actor Actor, in MintInput) (*MintedCredential, error)
	ListCredentials(ctx context.Context, f CredentialFilter) (*CredentialList, error)
	GetCredential(ctx context.Context, id string) (*models.MCPCredentialResponse, error)
	RotateCredential(ctx context.Context, actor Actor, id string) (*MintedCredential, error)
	SuspendCredential(ctx context.Context, actor Actor, id string, reason string) (*models.MCPCredentialResponse, error)
	ResumeCredential(ctx context.Context, actor Actor, id string) (*models.MCPCredentialResponse, error)
	RevokeCredential(ctx context.Context, actor Actor, id string, reason string) (*models.MCPCredentialResponse, error)
	// ApplyDrift applies a pending widening change.
	ApplyDrift(ctx context.Context, actor Actor, id string) (*models.MCPCredentialResponse, error)
	AccessReport(ctx context.Context, f ReportFilter) ([]AccessReportRow, error)

	// Lifecycle
	Status() Status
	Stop()
}
