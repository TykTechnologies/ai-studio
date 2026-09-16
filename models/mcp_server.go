package models

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

// MCP server kinds: what the Tyk proxy fronts.
const (
	MCPServerKindRemote    = "remote"      // upstream.url is a remote MCP endpoint
	MCPServerKindRestToMCP = "rest_to_mcp" // upstream.url is tyk://<api-id>/mcp
)

// Dashboard-side state of an MCP server as seen by the last sync.
const (
	MCPDashboardActive          = "active"
	MCPDashboardInactive        = "inactive"
	MCPDashboardMissing         = "missing"          // no longer returned by the Dashboard
	MCPDashboardPendingPlatform = "pending_platform" // handoff: waiting for the platform team to create the proxy
)

// Where an MCP server record came from.
const (
	MCPOriginDashboard  = "dashboard"
	MCPOriginStudio     = "studio"
	MCPOriginSubmission = "submission"
)

// Consumer authentication modes derived from the proxy definition.
const (
	MCPAuthKeyless       = "keyless"
	MCPAuthToken         = "auth_token"
	MCPAuthBasic         = "basic"
	MCPAuthJWT           = "jwt"
	MCPAuthOAuthTyk      = "oauth_tyk"
	MCPAuthOAuthExternal = "oauth_external"
	MCPAuthOAuth21       = "oauth21"
	MCPAuthMTLS          = "mtls"
	MCPAuthHMAC          = "hmac"
	MCPAuthCustom        = "custom"
	MCPAuthMixed         = "mixed"
)

// MCPPrimitive is one tool, resource or prompt known to Studio.
type MCPPrimitive struct {
	Type        string                 `json:"type"` // tool | resource | prompt
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
	// Auth records per-primitive overrides from the definition.
	Auth *MCPPrimitiveAuth `json:"auth,omitempty"`
	// Source names the REST operation behind a REST-to-MCP tool.
	Source string `json:"source,omitempty"`
}

// MCPPrimitiveAuth is the per-primitive authentication override.
type MCPPrimitiveAuth struct {
	IgnoreAuthentication bool     `json:"ignore_authentication,omitempty"`
	Scopes               []string `json:"scopes,omitempty"`
}

// MCPAuthDetails describes how a client authenticates to the proxy. Never
// carries secrets.
type MCPAuthDetails struct {
	// Header, Query or Cookie name the token scheme reads (auth_token, basic, jwt).
	HeaderName string `json:"header_name,omitempty"`
	QueryName  string `json:"query_name,omitempty"`
	CookieName string `json:"cookie_name,omitempty"`
	// Schemes lists every enabled scheme name with its resolved type.
	Schemes []MCPAuthScheme `json:"schemes,omitempty"`
	// PRM is the OAuth 2.1 protected resource metadata when advertised.
	PRM *MCPProtectedResourceMetadata `json:"prm,omitempty"`
}

// MCPAuthScheme is one security scheme on the proxy.
type MCPAuthScheme struct {
	Name string `json:"name"`
	Type string `json:"type"` // an MCPAuth* value
}

// MCPProtectedResourceMetadata mirrors RFC 9728 as configured on the proxy.
type MCPProtectedResourceMetadata struct {
	Resource             string   `json:"resource,omitempty"`
	AuthorizationServers []string `json:"authorization_servers,omitempty"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
	AutoDeriveScopes     bool     `json:"auto_derive_scopes,omitempty"`
	WellKnownPath        string   `json:"well_known_path,omitempty"`
	// URL is the absolute metadata URL when the endpoint URL is known.
	URL string `json:"url,omitempty"`
}

// MCPGatewayTags mirrors x-tyk-api-gateway.server.gatewayTags.
type MCPGatewayTags struct {
	Enabled bool     `json:"enabled"`
	Tags    []string `json:"tags"`
}

// MCPServer is an MCP proxy managed by a Tyk Gateway, catalogued in AI
// Studio. Definition-derived columns follow the Dashboard on every sync;
// Studio-owned presentation and governance columns never do.
type MCPServer struct {
	gorm.Model
	ConnectionID *uint          `gorm:"index;uniqueIndex:idx_mcp_servers_conn_api" json:"connection_id"`
	Connection   *TykConnection `gorm:"foreignKey:ConnectionID" json:"-"`
	// TykAPIID is x-tyk-api-gateway.info.id; empty while pending_platform.
	TykAPIID string `gorm:"size:64;uniqueIndex:idx_mcp_servers_conn_api" json:"tyk_api_id"`

	Name            string `gorm:"size:200;not null" json:"name"`
	NameOverridden  bool   `json:"name_overridden"`
	Slug            string `gorm:"size:200;uniqueIndex" json:"slug"`
	Description     string `gorm:"size:2048" json:"description"`
	LongDescription string `gorm:"type:text" json:"long_description"`
	LogoURL         string `gorm:"size:2048" json:"logo_url"`
	TagsJSON        string `gorm:"column:tags;type:text" json:"-"`

	Kind            string `gorm:"size:16;not null;default:remote" json:"kind"`
	ListenPath      string `gorm:"size:512" json:"listen_path"`
	ListenPathStrip bool   `json:"listen_path_strip"`
	TransportPath   string `gorm:"size:512" json:"transport_path"`
	EndpointURL     string `gorm:"size:2048" json:"endpoint_url"`
	// EndpointURLsJSON is map[tag]url when the proxy is segmented.
	EndpointURLsJSON string `gorm:"column:endpoint_urls;type:text" json:"-"`
	// UpstreamURL is admin-only; never in portal responses.
	UpstreamURL string `gorm:"size:2048" json:"-"`
	SourceAPIID string `gorm:"size:64" json:"source_api_id"`

	AuthMode        string `gorm:"size:32;not null;default:custom" json:"auth_mode"`
	AuthDetailsJSON string `gorm:"column:auth_details;type:text" json:"-"`
	PrimitivesJSON  string `gorm:"column:primitives;type:text" json:"-"`
	GatewayTagsJSON string `gorm:"column:gateway_tags;type:text" json:"-"`

	// Definition is the last synced OAS document with upstream.authentication
	// values masked. DefinitionHash is over the unmasked canonical form.
	Definition     string `gorm:"type:text" json:"-"`
	DefinitionHash string `gorm:"size:64" json:"definition_hash"`
	DashboardState string `gorm:"size:20;index;not null;default:active" json:"dashboard_state"`
	Origin         string `gorm:"size:16;not null;default:dashboard" json:"origin"`
	SubmissionID   *uint  `gorm:"index" json:"submission_id"`

	// PrivacyScore is nil until an administrator sets it; publish refuses meanwhile.
	PrivacyScore *int `json:"privacy_score"`
	IsActive     bool `gorm:"index" json:"is_active"`
	Brokerable   bool `json:"brokerable"`

	OwnerUserID        uint `json:"owner_user_id"`
	CommunitySubmitted bool `json:"community_submitted"`

	LastSeenAt   *time.Time `json:"last_seen_at"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	LockVersion  int        `gorm:"not null;default:0" json:"lock_version"`

	Groups []Group `gorm:"many2many:mcp_server_groups;" json:"-"`
}

func (MCPServer) TableName() string { return "mcp_servers" }

// MCPServerGroup is the direct team grant row (visibility in the portal).
type MCPServerGroup struct {
	MCPServerID uint `gorm:"primaryKey;column:mcp_server_id" json:"mcp_server_id"`
	GroupID     uint `gorm:"primaryKey" json:"group_id"`
}

func (MCPServerGroup) TableName() string { return "mcp_server_groups" }

// Tags decodes the presentation tags.
func (m *MCPServer) Tags() []string {
	out := []string{}
	if strings.TrimSpace(m.TagsJSON) != "" {
		_ = json.Unmarshal([]byte(m.TagsJSON), &out)
	}
	return out
}

// SetTags encodes the presentation tags.
func (m *MCPServer) SetTags(tags []string) {
	if len(tags) == 0 {
		m.TagsJSON = ""
		return
	}
	b, _ := json.Marshal(tags)
	m.TagsJSON = string(b)
}

// AuthDetails decodes the authentication details.
func (m *MCPServer) AuthDetails() MCPAuthDetails {
	var out MCPAuthDetails
	if strings.TrimSpace(m.AuthDetailsJSON) != "" {
		_ = json.Unmarshal([]byte(m.AuthDetailsJSON), &out)
	}
	return out
}

// SetAuthDetails encodes the authentication details.
func (m *MCPServer) SetAuthDetails(d MCPAuthDetails) {
	b, _ := json.Marshal(d)
	m.AuthDetailsJSON = string(b)
}

// Primitives decodes the known primitives.
func (m *MCPServer) Primitives() []MCPPrimitive {
	out := []MCPPrimitive{}
	if strings.TrimSpace(m.PrimitivesJSON) != "" {
		_ = json.Unmarshal([]byte(m.PrimitivesJSON), &out)
	}
	return out
}

// SetPrimitives encodes the known primitives.
func (m *MCPServer) SetPrimitives(p []MCPPrimitive) {
	if len(p) == 0 {
		m.PrimitivesJSON = ""
		return
	}
	b, _ := json.Marshal(p)
	m.PrimitivesJSON = string(b)
}

// GatewayTags decodes the segmentation tags.
func (m *MCPServer) GatewayTags() MCPGatewayTags {
	out := MCPGatewayTags{Tags: []string{}}
	if strings.TrimSpace(m.GatewayTagsJSON) != "" {
		_ = json.Unmarshal([]byte(m.GatewayTagsJSON), &out)
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	return out
}

// SetGatewayTags encodes the segmentation tags.
func (m *MCPServer) SetGatewayTags(t MCPGatewayTags) {
	if !t.Enabled && len(t.Tags) == 0 {
		m.GatewayTagsJSON = ""
		return
	}
	b, _ := json.Marshal(t)
	m.GatewayTagsJSON = string(b)
}

// EndpointURLs decodes the per-tag endpoint URLs.
func (m *MCPServer) EndpointURLs() map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(m.EndpointURLsJSON) != "" {
		_ = json.Unmarshal([]byte(m.EndpointURLsJSON), &out)
	}
	return out
}

// SetEndpointURLs encodes the per-tag endpoint URLs.
func (m *MCPServer) SetEndpointURLs(u map[string]string) {
	if len(u) == 0 {
		m.EndpointURLsJSON = ""
		return
	}
	b, _ := json.Marshal(u)
	m.EndpointURLsJSON = string(b)
}

// CanPublish reports whether the server may be made visible in the portal.
func (m *MCPServer) CanPublish() (bool, string) {
	if m.DashboardState != MCPDashboardActive {
		return false, "the MCP proxy is not active on the Tyk Dashboard"
	}
	if m.PrivacyScore == nil {
		return false, "an administrator must set a privacy score before publishing"
	}
	return true, ""
}

// MCPServerResponse is the administrator API shape. Upstream URL and the
// definition are admin-only; the portal projection lives in the catalogue.
type MCPServerResponse struct {
	ID                 uint              `json:"id"`
	ConnectionID       *uint             `json:"connection_id"`
	ConnectionName     string            `json:"connection_name,omitempty"`
	TykAPIID           string            `json:"tyk_api_id"`
	Name               string            `json:"name"`
	NameOverridden     bool              `json:"name_overridden"`
	Slug               string            `json:"slug"`
	Description        string            `json:"description"`
	LongDescription    string            `json:"long_description"`
	LogoURL            string            `json:"logo_url"`
	Tags               []string          `json:"tags"`
	Kind               string            `json:"kind"`
	ListenPath         string            `json:"listen_path"`
	TransportPath      string            `json:"transport_path"`
	EndpointURL        string            `json:"endpoint_url"`
	EndpointURLs       map[string]string `json:"endpoint_urls"`
	UpstreamURL        string            `json:"upstream_url"`
	SourceAPIID        string            `json:"source_api_id,omitempty"`
	AuthMode           string            `json:"auth_mode"`
	AuthDetails        MCPAuthDetails    `json:"auth_details"`
	Primitives         []MCPPrimitive    `json:"primitives"`
	GatewayTags        MCPGatewayTags    `json:"gateway_tags"`
	DefinitionHash     string            `json:"definition_hash"`
	DashboardState     string            `json:"dashboard_state"`
	Origin             string            `json:"origin"`
	SubmissionID       *uint             `json:"submission_id,omitempty"`
	PrivacyScore       *int              `json:"privacy_score"`
	IsActive           bool              `json:"is_active"`
	Brokerable         bool              `json:"brokerable"`
	OwnerUserID        uint              `json:"owner_user_id"`
	CommunitySubmitted bool              `json:"community_submitted"`
	LastSeenAt         *time.Time        `json:"last_seen_at,omitempty"`
	LastSyncedAt       *time.Time        `json:"last_synced_at,omitempty"`
	LockVersion        int               `json:"lock_version"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	// Detail-only fields.
	Definition string                  `json:"definition,omitempty"`
	GroupIDs   []uint                  `json:"group_ids,omitempty"`
	Bundle     []MCPServerPolicyPinView `json:"bundle,omitempty"`
}

// ToResponse converts the server to its administrator API shape.
func (m *MCPServer) ToResponse(detail bool) MCPServerResponse {
	r := MCPServerResponse{
		ID: m.ID, ConnectionID: m.ConnectionID, TykAPIID: m.TykAPIID,
		Name: m.Name, NameOverridden: m.NameOverridden, Slug: m.Slug,
		Description: m.Description, LongDescription: m.LongDescription, LogoURL: m.LogoURL, Tags: m.Tags(),
		Kind: m.Kind, ListenPath: m.ListenPath, TransportPath: m.TransportPath,
		EndpointURL: m.EndpointURL, EndpointURLs: m.EndpointURLs(), UpstreamURL: m.UpstreamURL, SourceAPIID: m.SourceAPIID,
		AuthMode: m.AuthMode, AuthDetails: m.AuthDetails(), Primitives: m.Primitives(), GatewayTags: m.GatewayTags(),
		DefinitionHash: m.DefinitionHash, DashboardState: m.DashboardState, Origin: m.Origin, SubmissionID: m.SubmissionID,
		PrivacyScore: m.PrivacyScore, IsActive: m.IsActive, Brokerable: m.Brokerable,
		OwnerUserID: m.OwnerUserID, CommunitySubmitted: m.CommunitySubmitted,
		LastSeenAt: m.LastSeenAt, LastSyncedAt: m.LastSyncedAt, LockVersion: m.LockVersion,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.Connection != nil {
		r.ConnectionName = m.Connection.Name
	}
	if detail {
		r.Definition = m.Definition
	}
	return r
}
