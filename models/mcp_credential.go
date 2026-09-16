package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// Credential status. minting/active/suspended/rotating_out are "live"
// (they occupy the one-per-App-per-connection slot); revoked and failed
// are terminal.
const (
	MCPCredentialMinting     = "minting"
	MCPCredentialActive      = "active"
	MCPCredentialSuspended   = "suspended"
	MCPCredentialRotatingOut = "rotating_out"
	MCPCredentialRevoked     = "revoked"
	MCPCredentialFailed      = "failed"
)

// Drift between the policies the key should carry and those it does.
const (
	MCPDriftNone         = "none"
	MCPDriftPendingWiden = "pending_widen"
	MCPDriftApplying     = "applying"
	MCPDriftError        = "error"
)

// How a revocation reached the Dashboard.
const (
	MCPRevokeModeDeleted      = "deleted"
	MCPRevokeModeInactiveOnly = "inactive_only"
)

// Credential purposes. "app" is the key handed to the App owner; "chat"
// is reserved for a Studio-held key (later milestone).
const (
	MCPCredentialPurposeApp  = "app"
	MCPCredentialPurposeChat = "chat"
)

// MCPCredentialLiveStatuses are the statuses that hold the per-App slot.
var MCPCredentialLiveStatuses = []string{MCPCredentialMinting, MCPCredentialActive, MCPCredentialSuspended, MCPCredentialRotatingOut}

// MCPCredential is the ledger row for one Tyk key Studio minted for an App
// on one connection. The key's plaintext is never stored: it is returned
// once by the mint call. TykKeyPlainToken is only used when the Dashboard
// runs with key hashing disabled, where the key id is the plaintext and is
// needed to address the key; it is encrypted at rest.
type MCPCredential struct {
	ID           string         `gorm:"primaryKey;size:36" json:"id"`
	AppID        uint           `gorm:"index;uniqueIndex:idx_mcp_cred_live,where:status <> 'revoked' AND status <> 'failed' AND status <> 'rotating_out'" json:"app_id"`
	ConnectionID uint           `gorm:"index;uniqueIndex:idx_mcp_cred_live,where:status <> 'revoked' AND status <> 'failed' AND status <> 'rotating_out'" json:"connection_id"`
	Purpose      string         `gorm:"size:16;not null;default:app;uniqueIndex:idx_mcp_cred_live,where:status <> 'revoked' AND status <> 'failed' AND status <> 'rotating_out'" json:"purpose"`
	App          *App           `gorm:"foreignKey:AppID" json:"-"`
	Connection   *TykConnection `gorm:"foreignKey:ConnectionID" json:"-"`

	Status string `gorm:"size:16;index;not null" json:"status"`

	TykKeyHash       string `gorm:"size:128;index" json:"tyk_key_hash"`
	TykKeyPlainToken string `gorm:"type:text" json:"-"`
	Alias            string `gorm:"size:255" json:"alias"`

	AppliedPolicyIDsJSON  string `gorm:"column:applied_policy_ids;type:text" json:"-"`
	ExternalPolicyIDsJSON string `gorm:"column:external_policy_ids;type:text" json:"-"`
	DesiredPolicyIDsJSON  string `gorm:"column:desired_policy_ids;type:text" json:"-"`
	Drift                 string `gorm:"size:16;index;not null;default:none" json:"drift"`
	DriftDetail           string `gorm:"size:1024" json:"drift_detail,omitempty"`

	ExpiresAt *time.Time `json:"expires_at"`

	MintedByUserID   uint       `json:"minted_by_user_id"`
	MintedAt         *time.Time `json:"minted_at"`
	RevealedToUserID uint       `json:"revealed_to_user_id"`
	RevealedAt       *time.Time `json:"revealed_at"`
	RevokedByUserID  uint       `json:"revoked_by_user_id"`
	RevokedAt        *time.Time `json:"revoked_at"`
	RevokeReason     string     `gorm:"size:255" json:"revoke_reason,omitempty"`
	RevokeMode       string     `gorm:"size:16" json:"revoke_mode,omitempty"`

	LastSyncedAt *time.Time `json:"last_synced_at"`
	LastError    string     `gorm:"size:1024" json:"last_error,omitempty"`
	LockVersion  int        `gorm:"not null;default:0" json:"lock_version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (MCPCredential) TableName() string { return "mcp_credentials" }

// BeforeSave encrypts the plaintext key id (unhashed Dashboards only),
// refusing to store it when the encryption key is not configured.
func (c *MCPCredential) BeforeSave(tx *gorm.DB) error {
	var err error
	if c.TykKeyPlainToken, err = tykEncrypt(c.TykKeyPlainToken); err != nil {
		return err
	}
	if c.Purpose == "" {
		c.Purpose = MCPCredentialPurposeApp
	}
	if c.Drift == "" {
		c.Drift = MCPDriftNone
	}
	return nil
}

// AfterSave restores plaintext on the in-memory struct.
func (c *MCPCredential) AfterSave(tx *gorm.DB) error { return c.AfterFind(tx) }

// AfterFind decrypts the plaintext key id.
func (c *MCPCredential) AfterFind(tx *gorm.DB) error {
	c.TykKeyPlainToken = secrets.DecryptValue(c.TykKeyPlainToken)
	return nil
}

func decodeStrings(s string) []string {
	out := []string{}
	if strings.TrimSpace(s) != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func encodeStrings(v []string) string {
	if len(v) == 0 {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// AppliedPolicyIDs are the Studio-owned policy ids the key carries.
func (c *MCPCredential) AppliedPolicyIDs() []string { return decodeStrings(c.AppliedPolicyIDsJSON) }
func (c *MCPCredential) SetAppliedPolicyIDs(ids []string) {
	c.AppliedPolicyIDsJSON = encodeStrings(ids)
}
func (c *MCPCredential) ExternalPolicyIDs() []string { return decodeStrings(c.ExternalPolicyIDsJSON) }
func (c *MCPCredential) SetExternalPolicyIDs(ids []string) {
	c.ExternalPolicyIDsJSON = encodeStrings(ids)
}
func (c *MCPCredential) DesiredPolicyIDs() []string { return decodeStrings(c.DesiredPolicyIDsJSON) }
func (c *MCPCredential) SetDesiredPolicyIDs(ids []string) {
	c.DesiredPolicyIDsJSON = encodeStrings(ids)
}

// IsLive reports whether the credential occupies the App's slot.
func (c *MCPCredential) IsLive() bool {
	for _, s := range MCPCredentialLiveStatuses {
		if c.Status == s {
			return true
		}
	}
	return false
}

// KeyRef returns the identifier to address the key on the Dashboard and
// whether it is a hash.
func (c *MCPCredential) KeyRef() (string, bool) {
	if c.TykKeyHash != "" {
		return c.TykKeyHash, true
	}
	return c.TykKeyPlainToken, false
}

// MCPCredentialResponse is the API shape of a credential. It never carries
// the key.
type MCPCredentialResponse struct {
	ID                string     `json:"id"`
	AppID             uint       `json:"app_id"`
	AppName           string     `json:"app_name,omitempty"`
	ConnectionID      uint       `json:"connection_id"`
	ConnectionName    string     `json:"connection_name,omitempty"`
	Purpose           string     `json:"purpose"`
	Status            string     `json:"status"`
	TykKeyHash        string     `json:"tyk_key_hash"`
	KeyHint           string     `json:"key_hint,omitempty"`
	Alias             string     `json:"alias"`
	AppliedPolicyIDs  []string   `json:"applied_policy_ids"`
	ExternalPolicyIDs []string   `json:"external_policy_ids"`
	DesiredPolicyIDs  []string   `json:"desired_policy_ids"`
	Drift             string     `json:"drift"`
	DriftDetail       string     `json:"drift_detail,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	MintedByUserID    uint       `json:"minted_by_user_id"`
	MintedAt          *time.Time `json:"minted_at,omitempty"`
	RevealedToUserID  uint       `json:"revealed_to_user_id"`
	RevealedAt        *time.Time `json:"revealed_at,omitempty"`
	RevokedByUserID   uint       `json:"revoked_by_user_id,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	RevokeReason      string     `json:"revoke_reason,omitempty"`
	RevokeMode        string     `json:"revoke_mode,omitempty"`
	LastSyncedAt      *time.Time `json:"last_synced_at,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	LockVersion       int        `json:"lock_version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ToResponse converts the credential to its API shape.
func (c *MCPCredential) ToResponse() MCPCredentialResponse {
	r := MCPCredentialResponse{
		ID: c.ID, AppID: c.AppID, ConnectionID: c.ConnectionID, Purpose: c.Purpose, Status: c.Status,
		TykKeyHash: c.TykKeyHash, KeyHint: tokenHint(c.TykKeyPlainToken), Alias: c.Alias,
		AppliedPolicyIDs: c.AppliedPolicyIDs(), ExternalPolicyIDs: c.ExternalPolicyIDs(), DesiredPolicyIDs: c.DesiredPolicyIDs(),
		Drift: c.Drift, DriftDetail: c.DriftDetail, ExpiresAt: c.ExpiresAt,
		MintedByUserID: c.MintedByUserID, MintedAt: c.MintedAt, RevealedToUserID: c.RevealedToUserID, RevealedAt: c.RevealedAt,
		RevokedByUserID: c.RevokedByUserID, RevokedAt: c.RevokedAt, RevokeReason: c.RevokeReason, RevokeMode: c.RevokeMode,
		LastSyncedAt: c.LastSyncedAt, LastError: c.LastError, LockVersion: c.LockVersion, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.App != nil {
		r.AppName = c.App.Name
	}
	if c.Connection != nil {
		r.ConnectionName = c.Connection.Name
	}
	return r
}
