package models

import "time"

// TykConnectionIngest is a Tyk connection's analytics ingest credential and
// state: Tyk Pump authenticates to the ingest endpoint with the token, and
// the connection the token belongs to scopes every record it delivers.
// There is at most one row per connection.
type TykConnectionIngest struct {
	ConnectionID uint `gorm:"primaryKey;autoIncrement:false" json:"connection_id"`
	// TokenHash is the SHA-256 (hex) of the ingest token; the plaintext is
	// never stored. Nil when no token is set (never generated, or revoked):
	// the unique index allows any number of NULLs.
	TokenHash *string `gorm:"size:64;uniqueIndex:idx_tyk_ingest_token_hash" json:"-"`
	// TokenPrefix is the start of the token, to tell tokens apart in the UI.
	TokenPrefix    string     `gorm:"size:32" json:"token_prefix"`
	TokenCreatedAt *time.Time `json:"token_created_at"`
	TokenCreatedBy uint       `json:"token_created_by"`
	LastBatchAt    *time.Time `json:"last_batch_at"`
	LastUnknownAPI string     `gorm:"column:last_unknown_api_id;size:64" json:"last_unknown_api_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TykIngestStat counts what one connection's ingest received in one hour,
// so the status can report a rolling 24 hours with one upsert per batch
// rather than a write per record.
type TykIngestStat struct {
	ConnectionID  uint      `gorm:"primaryKey;autoIncrement:false" json:"connection_id"`
	Hour          time.Time `gorm:"primaryKey;index:idx_tyk_ingest_stats_hour" json:"hour"`
	Received      int64     `gorm:"not null;default:0" json:"received"`
	Stored        int64     `gorm:"not null;default:0" json:"stored"`
	SkippedNonMCP int64     `gorm:"column:skipped_non_mcp;not null;default:0" json:"skipped_non_mcp"`
	UnknownAPI    int64     `gorm:"column:unknown_api;not null;default:0" json:"unknown_api"`
	Unattributed  int64     `gorm:"not null;default:0" json:"unattributed"`
}
