package models

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// Tyk connection trust modes. Studio runs at the lower of the declared mode
// and what the capability probe can verify against the Dashboard user's
// permissions.
const (
	TykConnectionModeCatalogue = "catalogue" // list/import proxies and policies only
	TykConnectionModeBroker    = "broker"    // + mint/update/revoke keys against pinned policies
	TykConnectionModeFull      = "full"      // + create/update proxies and policies
)

// Tyk connection lifecycle.
const (
	TykConnectionPending  = "pending"
	TykConnectionActive   = "active"
	TykConnectionDisabled = "disabled"
)

// TykConnectionModes is every mode in ascending order of trust.
var TykConnectionModes = []string{TykConnectionModeCatalogue, TykConnectionModeBroker, TykConnectionModeFull}

// TykModeRank orders modes so "lower of the two" is a comparison.
func TykModeRank(mode string) int {
	for i, m := range TykConnectionModes {
		if m == mode {
			return i
		}
	}
	return -1
}

// ErrSecretsKeyRequired is returned by the model hooks when a secret-bearing
// column would be written without the encryption key configured. The
// integration never stores a Dashboard credential in plaintext.
var ErrSecretsKeyRequired = errors.New("TYK_AI_SECRET_KEY must be configured before storing Tyk Dashboard credentials")

// TykConnection is one Tyk Dashboard (one organisation) that AI Studio
// imports MCP proxies from, registers MCP proxies into, and brokers access
// keys against.
//
// Secret-bearing columns carry "token" in their name so the audit trail's
// built-in redaction masks them in diffs, and they are encrypted at rest by
// the BeforeSave/AfterFind hooks, which refuse to save without the key.
type TykConnection struct {
	gorm.Model
	Name        string `gorm:"size:200;not null" json:"name"`
	Description string `gorm:"size:1024" json:"description"`

	DashboardURL   string `gorm:"size:2048;not null" json:"dashboard_url"`
	GatewayBaseURL string `gorm:"size:2048" json:"gateway_base_url"`
	// DashboardAccessToken is the Dashboard user's API access key. Encrypted.
	DashboardAccessToken string `gorm:"type:text" json:"-"`
	OrgID                string `gorm:"size:64" json:"org_id"`

	DeclaredMode  string `gorm:"size:16;not null;default:catalogue" json:"declared_mode"`
	EffectiveMode string `gorm:"size:16;not null;default:catalogue" json:"effective_mode"`
	// CapabilitiesJSON is a JSON object of capability name -> TykCapability.
	CapabilitiesJSON string `gorm:"column:capabilities;type:text" json:"-"`

	Status         string `gorm:"size:16;index;not null;default:pending" json:"status"`
	Degraded       bool   `json:"degraded"`
	DegradedReason string `gorm:"size:1024" json:"degraded_reason"`

	SyncIntervalSeconds int        `gorm:"not null;default:300" json:"sync_interval_seconds"`
	NextSyncAt          *time.Time `gorm:"index" json:"next_sync_at"`
	SyncLeaseOwner      string     `gorm:"size:255" json:"-"`
	SyncLeaseUntil      *time.Time `json:"-"`

	AutoPublish         bool `json:"auto_publish"`
	DefaultPrivacyScore *int `json:"default_privacy_score"`
	AcceptHandoffs      bool `gorm:"not null;default:true" json:"accept_handoffs"`

	// KeyDefaultsJSON holds TykKeyDefaults.
	KeyDefaultsJSON string `gorm:"column:key_defaults;type:text" json:"-"`

	AllowInternalHost bool `json:"allow_internal_host"`

	// Optional MDCB section for gateway segmentation discovery.
	MDCBURL               string `gorm:"size:2048" json:"mdcb_url"`
	MDCBAccessToken       string `gorm:"type:text" json:"-"`
	MDCBAllowInternalHost bool   `json:"mdcb_allow_internal_host"`
	// KnownGatewayTagsJSON holds []TykGatewayTag maintained by administrators.
	KnownGatewayTagsJSON string `gorm:"column:known_gateway_tags;type:text" json:"-"`
	// GatewayBaseURLsJSON holds map[tag]url.
	GatewayBaseURLsJSON string `gorm:"column:gateway_base_urls;type:text" json:"-"`
	// DataPlanesJSON holds []TykDataPlane, the sanitised MDCB snapshot.
	DataPlanesJSON string `gorm:"column:data_planes;type:text" json:"-"`

	LastSyncAt      *time.Time `json:"last_sync_at"`
	LastSyncStatus  string     `gorm:"size:16" json:"last_sync_status"`
	LastSyncError   string     `gorm:"size:2048" json:"last_sync_error"`
	LastProbeAt     *time.Time `json:"last_probe_at"`
	LastMDCBProbeAt *time.Time `json:"last_mdcb_probe_at"`

	CreatedByUserID   uint       `json:"created_by_user_id"`
	CreatedByEmail    string     `gorm:"size:255" json:"created_by_email"`
	ActivatedByUserID uint       `json:"activated_by_user_id"`
	ActivatedByEmail  string     `gorm:"size:255" json:"activated_by_email"`
	ActivatedAt       *time.Time `json:"activated_at"`

	LockVersion int `gorm:"not null;default:0" json:"lock_version"`
}

func (TykConnection) TableName() string { return "tyk_connections" }

// Capability states recorded by the probe.
const (
	TykCapabilityOK         = "ok"
	TykCapabilityDenied     = "denied"
	TykCapabilityUnverified = "unverified"
	TykCapabilityNo         = "no"
)

// Capability names.
const (
	TykCapMCPRead         = "mcp_read"
	TykCapPoliciesRead    = "policies_read"
	TykCapAPIsRead        = "apis_read"
	TykCapMCPSupported    = "mcp_supported"
	TykCapRestToMCP       = "rest_to_mcp_supported"
	TykCapKeysWrite       = "keys_write"
	TykCapKeysReadByHash  = "keys_read_by_hash"
	TykCapMCPWrite        = "mcp_write"
	TykCapPoliciesWrite   = "policies_write"
	TykCapKeyDeleteByHash = "key_delete_by_hash"
	TykCapMDCBRead        = "mdcb_read"
	// TykCapMCPDryRun records whether POST /api/mcps?dryRun=true really
	// validates without persisting. Dashboard 5.14 ignores the flag and
	// creates the proxy; Studio detects that, deletes it, and validates
	// locally from then on.
	TykCapMCPDryRun        = "mcp_dry_run"
	TykCapDashboardVersion = "dashboard_version"
)

// TykCapability is one probed capability.
type TykCapability struct {
	State     string     `json:"state"`
	Detail    string     `json:"detail,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// TykKeyDefaults are applied to every key minted on the connection.
type TykKeyDefaults struct {
	ExpiresInSeconds  int64  `json:"expires_in_seconds"`
	AliasPrefix       string `json:"alias_prefix"`
	DetailedRecording bool   `json:"detailed_recording"`
}

// TykGatewayTag is an administrator-maintained segmentation tag.
type TykGatewayTag struct {
	Tag         string `json:"tag"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// TykDataPlane is the sanitised view of one MDCB data plane group. The MDCB
// response also carries each node's gateway api_key; it is never stored.
type TykDataPlane struct {
	GroupID      string    `json:"group_id"`
	Tags         []string  `json:"tags"`
	NodeCount    int       `json:"node_count"`
	NodeVersions []string  `json:"node_versions"`
	Healthy      bool      `json:"healthy"`
	LastSeen     time.Time `json:"last_seen"`
}

const tykEncryptedPrefix = "$ENC/"

func tykEncrypt(v string) (string, error) {
	if v == "" || strings.HasPrefix(v, tykEncryptedPrefix) {
		return v, nil
	}
	if !secrets.EncryptionKeyConfigured() {
		return "", ErrSecretsKeyRequired
	}
	return secrets.EncryptValue(v), nil
}

// BeforeSave encrypts the access tokens. It refuses to save a plaintext token
// when the encryption key is not configured rather than falling back to
// plaintext storage.
func (t *TykConnection) BeforeSave(tx *gorm.DB) error {
	var err error
	if t.DashboardAccessToken, err = tykEncrypt(t.DashboardAccessToken); err != nil {
		return err
	}
	if t.MDCBAccessToken, err = tykEncrypt(t.MDCBAccessToken); err != nil {
		return err
	}
	if t.DeclaredMode == "" {
		t.DeclaredMode = TykConnectionModeCatalogue
	}
	if t.EffectiveMode == "" {
		t.EffectiveMode = TykConnectionModeCatalogue
	}
	if t.Status == "" {
		t.Status = TykConnectionPending
	}
	return nil
}

// AfterSave restores plaintext on the in-memory struct.
func (t *TykConnection) AfterSave(tx *gorm.DB) error { return t.AfterFind(tx) }

// AfterFind decrypts the access tokens.
func (t *TykConnection) AfterFind(tx *gorm.DB) error {
	t.DashboardAccessToken = secrets.DecryptValue(t.DashboardAccessToken)
	t.MDCBAccessToken = secrets.DecryptValue(t.MDCBAccessToken)
	return nil
}

// Capabilities decodes the probe results.
func (t *TykConnection) Capabilities() map[string]TykCapability {
	out := map[string]TykCapability{}
	if strings.TrimSpace(t.CapabilitiesJSON) != "" {
		_ = json.Unmarshal([]byte(t.CapabilitiesJSON), &out)
	}
	return out
}

// SetCapabilities encodes the probe results.
func (t *TykConnection) SetCapabilities(c map[string]TykCapability) {
	if len(c) == 0 {
		t.CapabilitiesJSON = ""
		return
	}
	b, _ := json.Marshal(c)
	t.CapabilitiesJSON = string(b)
}

// KeyDefaults decodes the key defaults.
func (t *TykConnection) KeyDefaults() TykKeyDefaults {
	out := TykKeyDefaults{AliasPrefix: "studio:"}
	if strings.TrimSpace(t.KeyDefaultsJSON) != "" {
		_ = json.Unmarshal([]byte(t.KeyDefaultsJSON), &out)
	}
	if out.AliasPrefix == "" {
		out.AliasPrefix = "studio:"
	}
	return out
}

// SetKeyDefaults encodes the key defaults.
func (t *TykConnection) SetKeyDefaults(d TykKeyDefaults) {
	b, _ := json.Marshal(d)
	t.KeyDefaultsJSON = string(b)
}

// KnownGatewayTags decodes the administrator-maintained tags.
func (t *TykConnection) KnownGatewayTags() []TykGatewayTag {
	out := []TykGatewayTag{}
	if strings.TrimSpace(t.KnownGatewayTagsJSON) != "" {
		_ = json.Unmarshal([]byte(t.KnownGatewayTagsJSON), &out)
	}
	return out
}

// SetKnownGatewayTags encodes the administrator-maintained tags.
func (t *TykConnection) SetKnownGatewayTags(tags []TykGatewayTag) {
	if len(tags) == 0 {
		t.KnownGatewayTagsJSON = ""
		return
	}
	b, _ := json.Marshal(tags)
	t.KnownGatewayTagsJSON = string(b)
}

// GatewayBaseURLs decodes the per-tag public base URLs.
func (t *TykConnection) GatewayBaseURLs() map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(t.GatewayBaseURLsJSON) != "" {
		_ = json.Unmarshal([]byte(t.GatewayBaseURLsJSON), &out)
	}
	return out
}

// SetGatewayBaseURLs encodes the per-tag public base URLs.
func (t *TykConnection) SetGatewayBaseURLs(m map[string]string) {
	if len(m) == 0 {
		t.GatewayBaseURLsJSON = ""
		return
	}
	b, _ := json.Marshal(m)
	t.GatewayBaseURLsJSON = string(b)
}

// DataPlanes decodes the sanitised MDCB snapshot.
func (t *TykConnection) DataPlanes() []TykDataPlane {
	out := []TykDataPlane{}
	if strings.TrimSpace(t.DataPlanesJSON) != "" {
		_ = json.Unmarshal([]byte(t.DataPlanesJSON), &out)
	}
	return out
}

// SetDataPlanes encodes the sanitised MDCB snapshot.
func (t *TykConnection) SetDataPlanes(d []TykDataPlane) {
	if len(d) == 0 {
		t.DataPlanesJSON = ""
		return
	}
	b, _ := json.Marshal(d)
	t.DataPlanesJSON = string(b)
}

// AllGatewayTags is the union of MDCB-discovered and administrator-known tags.
func (t *TykConnection) AllGatewayTags() []string {
	seen := map[string]bool{}
	var out []string
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		out = append(out, tag)
	}
	for _, dp := range t.DataPlanes() {
		for _, tag := range dp.Tags {
			add(tag)
		}
	}
	for _, kt := range t.KnownGatewayTags() {
		add(kt.Tag)
	}
	return out
}

// HasMDCB reports whether the MDCB section is configured.
func (t *TykConnection) HasMDCB() bool {
	return strings.TrimSpace(t.MDCBURL) != ""
}

// IsUsable reports whether Studio may call the Dashboard for this connection.
func (t *TykConnection) IsUsable() bool {
	return t.Status == TykConnectionActive
}

// tokenHint returns the last four characters of a token, or "".
func tokenHint(v string) string {
	if len(v) < 8 {
		return ""
	}
	return v[len(v)-4:]
}

// TykConnectionResponse is the API shape of a connection. Tokens are never
// included; a presence flag and a last-four hint replace them.
type TykConnectionResponse struct {
	ID                  uint                     `json:"id"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	DashboardURL        string                   `json:"dashboard_url"`
	GatewayBaseURL      string                   `json:"gateway_base_url"`
	HasToken            bool                     `json:"has_token"`
	TokenHint           string                   `json:"token_hint,omitempty"`
	OrgID               string                   `json:"org_id"`
	DeclaredMode        string                   `json:"declared_mode"`
	EffectiveMode       string                   `json:"effective_mode"`
	Capabilities        map[string]TykCapability `json:"capabilities"`
	Status              string                   `json:"status"`
	Degraded            bool                     `json:"degraded"`
	DegradedReason      string                   `json:"degraded_reason,omitempty"`
	SyncIntervalSeconds int                      `json:"sync_interval_seconds"`
	NextSyncAt          *time.Time               `json:"next_sync_at,omitempty"`
	AutoPublish         bool                     `json:"auto_publish"`
	DefaultPrivacyScore *int                     `json:"default_privacy_score"`
	AcceptHandoffs      bool                     `json:"accept_handoffs"`
	KeyDefaults         TykKeyDefaults           `json:"key_defaults"`
	AllowInternalHost   bool                     `json:"allow_internal_host"`
	MDCBURL             string                   `json:"mdcb_url"`
	HasMDCBToken        bool                     `json:"has_mdcb_token"`
	MDCBAllowInternal   bool                     `json:"mdcb_allow_internal_host"`
	KnownGatewayTags    []TykGatewayTag          `json:"known_gateway_tags"`
	GatewayBaseURLs     map[string]string        `json:"gateway_base_urls"`
	DataPlanes          []TykDataPlane           `json:"data_planes"`
	GatewayTags         []string                 `json:"gateway_tags"`
	LastSyncAt          *time.Time               `json:"last_sync_at,omitempty"`
	LastSyncStatus      string                   `json:"last_sync_status,omitempty"`
	LastSyncError       string                   `json:"last_sync_error,omitempty"`
	LastProbeAt         *time.Time               `json:"last_probe_at,omitempty"`
	LastMDCBProbeAt     *time.Time               `json:"last_mdcb_probe_at,omitempty"`
	CreatedByUserID     uint                     `json:"created_by_user_id"`
	CreatedByEmail      string                   `json:"created_by_email"`
	ActivatedByUserID   uint                     `json:"activated_by_user_id"`
	ActivatedByEmail    string                   `json:"activated_by_email"`
	ActivatedAt         *time.Time               `json:"activated_at,omitempty"`
	LockVersion         int                      `json:"lock_version"`
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
}

// ToResponse converts the connection to its API shape, dropping every secret.
func (t *TykConnection) ToResponse() TykConnectionResponse {
	tags := t.AllGatewayTags()
	if tags == nil {
		tags = []string{}
	}
	return TykConnectionResponse{
		ID: t.ID, Name: t.Name, Description: t.Description,
		DashboardURL: t.DashboardURL, GatewayBaseURL: t.GatewayBaseURL,
		HasToken: t.DashboardAccessToken != "", TokenHint: tokenHint(t.DashboardAccessToken),
		OrgID: t.OrgID, DeclaredMode: t.DeclaredMode, EffectiveMode: t.EffectiveMode,
		Capabilities: t.Capabilities(), Status: t.Status, Degraded: t.Degraded, DegradedReason: t.DegradedReason,
		SyncIntervalSeconds: t.SyncIntervalSeconds, NextSyncAt: t.NextSyncAt,
		AutoPublish: t.AutoPublish, DefaultPrivacyScore: t.DefaultPrivacyScore, AcceptHandoffs: t.AcceptHandoffs,
		KeyDefaults: t.KeyDefaults(), AllowInternalHost: t.AllowInternalHost,
		MDCBURL: t.MDCBURL, HasMDCBToken: t.MDCBAccessToken != "", MDCBAllowInternal: t.MDCBAllowInternalHost,
		KnownGatewayTags: t.KnownGatewayTags(), GatewayBaseURLs: t.GatewayBaseURLs(), DataPlanes: t.DataPlanes(),
		GatewayTags: tags,
		LastSyncAt:  t.LastSyncAt, LastSyncStatus: t.LastSyncStatus, LastSyncError: t.LastSyncError,
		LastProbeAt: t.LastProbeAt, LastMDCBProbeAt: t.LastMDCBProbeAt,
		CreatedByUserID: t.CreatedByUserID, CreatedByEmail: t.CreatedByEmail,
		ActivatedByUserID: t.ActivatedByUserID, ActivatedByEmail: t.ActivatedByEmail, ActivatedAt: t.ActivatedAt,
		LockVersion: t.LockVersion, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}
