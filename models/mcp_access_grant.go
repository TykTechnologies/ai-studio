package models

import (
	"time"

	"gorm.io/gorm"
)

// Grant kinds: how the App reaches the MCP server.
const (
	MCPGrantKindKey      = "key"      // a Tyk key Studio mints (broker)
	MCPGrantKindOAuth    = "oauth"    // the proxy advertises OAuth; the client obtains its own token
	MCPGrantKindKeyless  = "keyless"  // no credential needed
	MCPGrantKindExternal = "external" // mTLS, HMAC, JWT or custom: provisioned outside Studio
)

// MCPAccessGrant records that an App (and so its owner) has access to an
// MCP server, whether or not Studio minted a key for it. One open row per
// (app, server); revoked rows are kept for the access report.
type MCPAccessGrant struct {
	gorm.Model
	AppID           uint       `gorm:"index;uniqueIndex:idx_mcp_grants_open,where:revoked_at IS NULL" json:"app_id"`
	MCPServerID     uint       `gorm:"column:mcp_server_id;index;uniqueIndex:idx_mcp_grants_open,where:revoked_at IS NULL" json:"mcp_server_id"`
	UserID          uint       `gorm:"index" json:"user_id"`
	GrantKind       string     `gorm:"size:16;not null" json:"grant_kind"`
	CredentialID    *string    `gorm:"size:36;index" json:"credential_id"`
	GrantedAt       time.Time  `json:"granted_at"`
	GrantedByUserID uint       `json:"granted_by_user_id"`
	RevokedAt       *time.Time `gorm:"index" json:"revoked_at"`
	RevokeReason    string     `gorm:"size:255" json:"revoke_reason,omitempty"`
}

func (MCPAccessGrant) TableName() string { return "mcp_access_grants" }

// IsOpen reports whether the grant is current.
func (g *MCPAccessGrant) IsOpen() bool { return g.RevokedAt == nil }

// GrantKindForAuthMode maps a server's consumer auth mode to the grant kind.
func GrantKindForAuthMode(authMode string) string {
	switch authMode {
	case MCPAuthKeyless:
		return MCPGrantKindKeyless
	case MCPAuthToken, MCPAuthBasic:
		return MCPGrantKindKey
	case MCPAuthOAuthTyk, MCPAuthOAuthExternal, MCPAuthOAuth21:
		return MCPGrantKindOAuth
	case MCPAuthMixed:
		return MCPGrantKindKey
	default:
		return MCPGrantKindExternal
	}
}
