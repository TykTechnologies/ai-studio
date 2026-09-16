package models

import "time"

// Sync run outcomes.
const (
	MCPSyncRunning = "running"
	MCPSyncOK      = "ok"
	MCPSyncPartial = "partial"
	MCPSyncFailed  = "failed"
)

// MCPSyncRun records one discovery sync of a connection. Append-only;
// pruned by retention.
type MCPSyncRun struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ConnectionID uint       `gorm:"index" json:"connection_id"`
	NodeID       string     `gorm:"size:255" json:"node_id"`
	StartedAt    time.Time  `gorm:"index" json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	Status       string     `gorm:"size:16" json:"status"`
	Trigger      string     `gorm:"size:16" json:"trigger"` // schedule | manual

	ProxiesSeen    int `json:"proxies_seen"`
	ProxiesAdded   int `json:"proxies_added"`
	ProxiesUpdated int `json:"proxies_updated"`
	ProxiesMissing int `json:"proxies_missing"`
	ProxiesResumed int `json:"proxies_resumed"`

	PoliciesSeen    int `json:"policies_seen"`
	PoliciesUpdated int `json:"policies_updated"`
	PoliciesMissing int `json:"policies_missing"`

	CredentialsChecked int `json:"credentials_checked"`
	CredentialsDrifted int `json:"credentials_drifted"`

	Error string `gorm:"size:2048" json:"error,omitempty"`
}

func (MCPSyncRun) TableName() string { return "mcp_sync_runs" }
