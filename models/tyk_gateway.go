package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A Tyk OSS Gateway has no Dashboard to hold the definitions and no API that
// spans the cluster: each node keeps the MCP proxies it was given in its own
// app_path. Studio therefore keeps the desired state of the proxies it owns
// (TykGatewayDefinition) and reconciles every node it finds (TykGatewayNode)
// to it. Keys need neither: they live in the Redis the nodes share.

// Gateway node sources.
const (
	TykGatewayNodeSourceURL    = "url"    // the connection URL
	TykGatewayNodeSourceStatic = "static" // the administrator's list
	TykGatewayNodeSourceDNS    = "dns"    // an address the connection URL's hostname resolved to
)

// Gateway node sync states.
const (
	TykGatewayNodeInSync      = "in_sync"
	TykGatewayNodePending     = "pending"     // reachable, but missing or holding a stale Studio proxy
	TykGatewayNodeUnreachable = "unreachable" // did not answer
	TykGatewayNodeGone        = "gone"        // no longer discovered
)

// TykGatewayNode is one node of a Tyk Gateway connection as Studio last saw it.
type TykGatewayNode struct {
	ID           uint `gorm:"primaryKey" json:"id"`
	ConnectionID uint `gorm:"not null;uniqueIndex:idx_tyk_gateway_nodes_conn_addr" json:"connection_id"`
	// Address identifies the node: the URL for url/static nodes, the
	// resolved ip:port for dns nodes.
	Address string `gorm:"size:512;not null;uniqueIndex:idx_tyk_gateway_nodes_conn_addr" json:"address"`
	// URL is the Gateway API base URL the node is called on. A dns node
	// shares the connection URL and is dialled at PinnedIP.
	URL      string `gorm:"size:2048;not null" json:"url"`
	PinnedIP string `gorm:"size:64" json:"pinned_ip,omitempty"`
	Source   string `gorm:"size:16;not null" json:"source"`

	State     string `gorm:"size:16;not null;default:pending" json:"state"`
	Reachable bool   `json:"reachable"`
	Version   string `gorm:"size:64" json:"version"`
	// MCPCount is how many MCP proxies the node serves; StudioCount how many
	// of them Studio owns; ExpectedCount how many Studio wants it to serve.
	MCPCount      int `json:"mcp_count"`
	StudioCount   int `json:"studio_count"`
	ExpectedCount int `json:"expected_count"`
	// AppliedJSON holds map[api id]TykGatewayApplied: what Studio last wrote
	// to the node and what the node returned for it, so an unchanged node is
	// not rewritten and a definition changed behind Studio's back is.
	AppliedJSON string `gorm:"column:applied;type:text" json:"-"`

	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	LastReconcileAt *time.Time `json:"last_reconcile_at"`
	LastError       string     `gorm:"size:2048" json:"last_error,omitempty"`
	GoneAt          *time.Time `json:"gone_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (TykGatewayNode) TableName() string { return "tyk_gateway_nodes" }

// TykGatewayApplied records one Studio proxy on one node.
type TykGatewayApplied struct {
	// Sent is the desired definition's hash when it was written.
	Sent string `json:"sent"`
	// Observed is the hash of the node's copy right after the write.
	Observed string `json:"observed"`
}

// Applied decodes the per-proxy write record.
func (n *TykGatewayNode) Applied() map[string]TykGatewayApplied {
	out := map[string]TykGatewayApplied{}
	if strings.TrimSpace(n.AppliedJSON) != "" {
		_ = json.Unmarshal([]byte(n.AppliedJSON), &out)
	}
	return out
}

// SetApplied encodes the per-proxy write record.
func (n *TykGatewayNode) SetApplied(m map[string]TykGatewayApplied) {
	if len(m) == 0 {
		n.AppliedJSON = ""
		return
	}
	b, _ := json.Marshal(m)
	n.AppliedJSON = string(b)
}

// TykGatewayDefinition is the desired state of one MCP proxy Studio owns on
// a Tyk Gateway connection: the full definition, upstream credentials
// included, encrypted at rest. It is the source of truth the reconciler
// writes to every node.
type TykGatewayDefinition struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	ConnectionID uint   `gorm:"not null;uniqueIndex:idx_tyk_gateway_definitions_conn_api" json:"connection_id"`
	TykAPIID     string `gorm:"size:64;not null;uniqueIndex:idx_tyk_gateway_definitions_conn_api" json:"tyk_api_id"`
	// Definition is the Tyk OAS document. Encrypted ("token" is not in the
	// name, so the field is excluded from JSON instead).
	Definition string `gorm:"type:text;not null" json:"-"`
	// DefinitionHash is the sha256 of the plaintext document.
	DefinitionHash string    `gorm:"size:64;not null" json:"definition_hash"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (TykGatewayDefinition) TableName() string { return "tyk_gateway_definitions" }

// BeforeSave encrypts the definition; it refuses to store plaintext.
func (d *TykGatewayDefinition) BeforeSave(tx *gorm.DB) error {
	if d.Definition == "" || strings.HasPrefix(d.Definition, tykEncryptedPrefix) {
		return nil
	}
	if !secrets.EncryptionKeyConfigured() {
		return ErrSecretsKeyRequired
	}
	d.Definition = secrets.EncryptValue(d.Definition)
	return nil
}

// AfterSave restores plaintext on the in-memory struct.
func (d *TykGatewayDefinition) AfterSave(tx *gorm.DB) error { return d.AfterFind(tx) }

// AfterFind decrypts the definition.
func (d *TykGatewayDefinition) AfterFind(tx *gorm.DB) error {
	d.Definition = secrets.DecryptValue(d.Definition)
	return nil
}

// TykGatewayNodeResponse is the API shape of a node.
type TykGatewayNodeResponse struct {
	ID              uint       `json:"id"`
	Address         string     `json:"address"`
	URL             string     `json:"url"`
	PinnedIP        string     `json:"pinned_ip,omitempty"`
	Source          string     `json:"source"`
	State           string     `json:"state"`
	Reachable       bool       `json:"reachable"`
	Version         string     `json:"version"`
	MCPCount        int        `json:"mcp_count"`
	StudioCount     int        `json:"studio_count"`
	ExpectedCount   int        `json:"expected_count"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	LastReconcileAt *time.Time `json:"last_reconcile_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	GoneAt          *time.Time `json:"gone_at,omitempty"`
}

// ToResponse converts a node to its API shape.
func (n *TykGatewayNode) ToResponse() TykGatewayNodeResponse {
	return TykGatewayNodeResponse{
		ID: n.ID, Address: n.Address, URL: n.URL, PinnedIP: n.PinnedIP, Source: n.Source,
		State: n.State, Reachable: n.Reachable, Version: n.Version,
		MCPCount: n.MCPCount, StudioCount: n.StudioCount, ExpectedCount: n.ExpectedCount,
		FirstSeenAt: n.FirstSeenAt, LastSeenAt: n.LastSeenAt, LastReconcileAt: n.LastReconcileAt,
		LastError: n.LastError, GoneAt: n.GoneAt,
	}
}
