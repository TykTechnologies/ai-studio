package models

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Policy cache state.
const (
	TykPolicyPresent = "present"
	TykPolicyMissing = "missing"
)

// Bundle pin roles: one access policy (ACL) and any number of consumption
// policies (rate limit / quota partitions), the Developer Portal shape.
const (
	MCPPinRoleAccess      = "access"
	MCPPinRoleConsumption = "consumption"
)

// TykPolicyPartitions mirrors a policy's partitions object.
type TykPolicyPartitions struct {
	ACL        bool `json:"acl"`
	RateLimit  bool `json:"rate_limit"`
	Quota      bool `json:"quota"`
	Complexity bool `json:"complexity"`
	PerAPI     bool `json:"per_api"`
}

// TykPolicy is a read-mostly cache of a Tyk security policy, refreshed on
// every sync. The full JSON is admin-only.
type TykPolicy struct {
	gorm.Model
	ConnectionID uint   `gorm:"index;uniqueIndex:idx_tyk_policies_conn_pol" json:"connection_id"`
	TykPolicyID  string `gorm:"size:64;uniqueIndex:idx_tyk_policies_conn_pol" json:"tyk_policy_id"`
	Name         string `gorm:"size:255" json:"name"`
	Active       bool   `json:"active"`
	IsInactive   bool   `json:"is_inactive"`
	// PartitionsJSON holds TykPolicyPartitions.
	PartitionsJSON string `gorm:"column:partitions;type:text" json:"-"`
	TagsJSON       string `gorm:"column:tags;type:text" json:"-"`
	KeyExpiresIn   int64  `json:"key_expires_in"`
	// MCPAPIIDsJSON lists the MCP proxy ids in access_rights.
	MCPAPIIDsJSON string `gorm:"column:mcp_api_ids;type:text" json:"-"`
	// APIIDsJSON lists every api id in access_rights (MCP or not).
	APIIDsJSON     string     `gorm:"column:api_ids;type:text" json:"-"`
	IsPartitioned  bool       `json:"is_partitioned"`
	HasACL         bool       `json:"has_acl"`
	HasRateLimit   bool       `json:"has_rate_limit"`
	HasQuota       bool       `json:"has_quota"`
	HasComplexity  bool       `json:"has_complexity"`
	HasPerAPI      bool       `json:"has_per_api"`
	StudioManaged  bool       `json:"studio_managed"`
	Raw            string     `gorm:"type:text" json:"-"`
	DashboardState string     `gorm:"size:16;not null;default:present" json:"dashboard_state"`
	LastSeenAt     *time.Time `json:"last_seen_at"`
}

func (TykPolicy) TableName() string { return "tyk_policies" }

// Partitions decodes the partition flags.
func (p *TykPolicy) Partitions() TykPolicyPartitions {
	var out TykPolicyPartitions
	if strings.TrimSpace(p.PartitionsJSON) != "" {
		_ = json.Unmarshal([]byte(p.PartitionsJSON), &out)
	}
	return out
}

// SetPartitions encodes the partition flags and the denormalised columns.
func (p *TykPolicy) SetPartitions(part TykPolicyPartitions) {
	b, _ := json.Marshal(part)
	p.PartitionsJSON = string(b)
	p.IsPartitioned = part.ACL || part.RateLimit || part.Quota || part.Complexity || part.PerAPI
	p.HasACL = part.ACL
	p.HasRateLimit = part.RateLimit
	p.HasQuota = part.Quota
	p.HasComplexity = part.Complexity
	p.HasPerAPI = part.PerAPI
}

// Tags decodes the policy tags.
func (p *TykPolicy) Tags() []string {
	out := []string{}
	if strings.TrimSpace(p.TagsJSON) != "" {
		_ = json.Unmarshal([]byte(p.TagsJSON), &out)
	}
	return out
}

// SetTags encodes the policy tags.
func (p *TykPolicy) SetTags(tags []string) {
	if len(tags) == 0 {
		p.TagsJSON = ""
		return
	}
	b, _ := json.Marshal(tags)
	p.TagsJSON = string(b)
}

// MCPAPIIDs decodes the MCP proxy ids the policy grants.
func (p *TykPolicy) MCPAPIIDs() []string {
	out := []string{}
	if strings.TrimSpace(p.MCPAPIIDsJSON) != "" {
		_ = json.Unmarshal([]byte(p.MCPAPIIDsJSON), &out)
	}
	return out
}

// SetMCPAPIIDs encodes the MCP proxy ids the policy grants.
func (p *TykPolicy) SetMCPAPIIDs(ids []string) {
	if len(ids) == 0 {
		p.MCPAPIIDsJSON = ""
		return
	}
	b, _ := json.Marshal(ids)
	p.MCPAPIIDsJSON = string(b)
}

// APIIDs decodes every api id the policy grants.
func (p *TykPolicy) APIIDs() []string {
	out := []string{}
	if strings.TrimSpace(p.APIIDsJSON) != "" {
		_ = json.Unmarshal([]byte(p.APIIDsJSON), &out)
	}
	return out
}

// SetAPIIDs encodes every api id the policy grants.
func (p *TykPolicy) SetAPIIDs(ids []string) {
	if len(ids) == 0 {
		p.APIIDsJSON = ""
		return
	}
	b, _ := json.Marshal(ids)
	p.APIIDsJSON = string(b)
}

// GrantsAPI reports whether the policy's access rights name the api id.
func (p *TykPolicy) GrantsAPI(apiID string) bool {
	for _, id := range p.APIIDs() {
		if id == apiID {
			return true
		}
	}
	return false
}

// IsAllInOne reports a non-partitioned policy (ACL and limits together).
func (p *TykPolicy) IsAllInOne() bool { return !p.IsPartitioned }

// TykPolicyResponse is the administrator API shape of a cached policy.
type TykPolicyResponse struct {
	ID             uint                `json:"id"`
	ConnectionID   uint                `json:"connection_id"`
	TykPolicyID    string              `json:"tyk_policy_id"`
	Name           string              `json:"name"`
	Active         bool                `json:"active"`
	IsInactive     bool                `json:"is_inactive"`
	Partitions     TykPolicyPartitions `json:"partitions"`
	IsPartitioned  bool                `json:"is_partitioned"`
	Tags           []string            `json:"tags"`
	KeyExpiresIn   int64               `json:"key_expires_in"`
	MCPAPIIDs      []string            `json:"mcp_api_ids"`
	APIIDs         []string            `json:"api_ids"`
	StudioManaged  bool                `json:"studio_managed"`
	DashboardState string              `json:"dashboard_state"`
	LastSeenAt     *time.Time          `json:"last_seen_at,omitempty"`
	Raw            json.RawMessage     `json:"raw,omitempty"`
}

// ToResponse converts the policy to its API shape; raw is included on detail.
func (p *TykPolicy) ToResponse(detail bool) TykPolicyResponse {
	r := TykPolicyResponse{
		ID: p.ID, ConnectionID: p.ConnectionID, TykPolicyID: p.TykPolicyID, Name: p.Name,
		Active: p.Active, IsInactive: p.IsInactive, Partitions: p.Partitions(), IsPartitioned: p.IsPartitioned,
		Tags: p.Tags(), KeyExpiresIn: p.KeyExpiresIn, MCPAPIIDs: p.MCPAPIIDs(), APIIDs: p.APIIDs(),
		StudioManaged: p.StudioManaged, DashboardState: p.DashboardState, LastSeenAt: p.LastSeenAt,
	}
	if detail && strings.TrimSpace(p.Raw) != "" {
		r.Raw = json.RawMessage(p.Raw)
	}
	return r
}

// MCPServerPolicyPin is one policy in a server's bundle.
type MCPServerPolicyPin struct {
	gorm.Model
	MCPServerID uint `gorm:"column:mcp_server_id;index;uniqueIndex:idx_mcp_pins_server_policy" json:"mcp_server_id"`
	// PolicyID is the tyk_policies row id (not the Tyk-side policy id).
	PolicyID      uint       `gorm:"index;uniqueIndex:idx_mcp_pins_server_policy" json:"policy_id"`
	TykPolicy     *TykPolicy `gorm:"foreignKey:PolicyID;references:ID" json:"-"`
	Role          string     `gorm:"size:16;not null" json:"role"`
	Position      int        `json:"position"`
	InvalidReason string     `gorm:"size:1024" json:"invalid_reason"`
}

func (MCPServerPolicyPin) TableName() string { return "mcp_server_policy_pins" }

// MCPServerPolicyPinView is the API shape of a pin with its policy summary.
type MCPServerPolicyPinView struct {
	ID            uint              `json:"id"`
	Role          string            `json:"role"`
	Position      int               `json:"position"`
	InvalidReason string            `json:"invalid_reason,omitempty"`
	Policy        TykPolicyResponse `json:"policy"`
}

// ToView converts a pin (with its policy preloaded) to the API shape.
func (p *MCPServerPolicyPin) ToView() MCPServerPolicyPinView {
	v := MCPServerPolicyPinView{ID: p.ID, Role: p.Role, Position: p.Position, InvalidReason: p.InvalidReason}
	if p.TykPolicy != nil {
		v.Policy = p.TykPolicy.ToResponse(false)
	}
	return v
}
