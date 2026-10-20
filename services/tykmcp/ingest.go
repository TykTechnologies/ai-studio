package tykmcp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// IngestPath is where Tyk Pump delivers analytics records.
const IngestPath = "/api/v1/ingest/tyk-analytics"

// ErrIngestUnauthorized is returned for an ingest request whose token is
// missing, unknown or revoked, or whose connection is disabled or deleted.
// The endpoint answers 401, so the pump drops the batch rather than
// retrying it.
var ErrIngestUnauthorized = errors.New("invalid ingest token")

// ErrIngestBusy is returned when a batch could not be recorded right now
// (the analytics recorder's buffer is full). The endpoint answers 503, so
// the pump retries the batch later.
var ErrIngestBusy = errors.New("analytics ingest is busy; retry later")

// IngestTokenPrefix starts every ingest token, so secret scanners and
// administrators can recognise one.
const IngestTokenPrefix = "tai_ing_"

// ingestTokenDisplayLen is how much of a token TokenPrefix keeps: the fixed
// prefix and six random characters.
const ingestTokenDisplayLen = len(IngestTokenPrefix) + 6

// NewIngestToken returns a new random ingest token, its hash (what is
// stored) and its display prefix.
func NewIngestToken() (token, hash, display string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", err
	}
	token = IngestTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, HashIngestToken(token), token[:ingestTokenDisplayLen], nil
}

// HashIngestToken is the stored form of an ingest token: SHA-256, hex. The
// token carries 256 random bits, so an unsalted fast hash is enough to make
// a leaked row useless.
func HashIngestToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IngestTokenMatches compares a presented token with a stored hash in
// constant time.
func IngestTokenMatches(token, storedHash string) bool {
	if token == "" || storedHash == "" {
		return false
	}
	got := HashIngestToken(token)
	return subtle.ConstantTimeCompare([]byte(got), []byte(strings.ToLower(storedHash))) == 1
}

// IngestResult is the ingest endpoint's answer for one batch.
type IngestResult struct {
	// Accepted is how many records the batch held.
	Accepted int `json:"accepted"`
	// Stored is how many were recorded as MCP calls.
	Stored int `json:"stored"`
	// SkippedNonMCP records were not MCP traffic.
	SkippedNonMCP int `json:"skipped_non_mcp"`
	// UnknownAPI records were MCP traffic for an API that is not an MCP
	// server on the token's connection; they are not stored.
	UnknownAPI int `json:"unknown_api"`
	// Unattributed records were stored without an App: the key is not a
	// brokered credential and the alias names no App.
	Unattributed int `json:"unattributed"`
}

// IngestStatus is what a connection's analytics ingest has seen.
type IngestStatus struct {
	ConnectionID   uint       `json:"connection_id"`
	HasToken       bool       `json:"has_token"`
	TokenPrefix    string     `json:"token_prefix"`
	TokenCreatedAt *time.Time `json:"token_created_at"`
	LastBatchAt    *time.Time `json:"last_batch_at"`
	// The counters cover the last 24 hours.
	Records24h       int64  `json:"records_24h"`
	UnknownAPI24h    int64  `json:"unknown_api_24h"`
	SkippedNonMCP24h int64  `json:"skipped_non_mcp_24h"`
	Unattributed24h  int64  `json:"unattributed_24h"`
	LastUnknownAPIID string `json:"last_unknown_api_id"`
	IngestPath       string `json:"ingest_path"`
}

// RotatedIngestToken is a newly generated ingest token. Token is the
// plaintext, returned exactly once.
type RotatedIngestToken struct {
	Token  string       `json:"token"`
	Status IngestStatus `json:"status"`
}
