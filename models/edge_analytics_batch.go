package models

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// EdgeAnalyticsBatch records that an edge's analytics batch (one analytics
// pulse) was stored, so the same batch sent again is not stored twice. An
// edge sends a batch again when the acknowledgement was lost, typically
// because the control plane that stored it went away before answering and
// the edge moved to another one. The row is written in the transaction that
// stores the batch's records. Rows are pruned after
// EdgeAnalyticsBatchRetention.
type EdgeAnalyticsBatch struct {
	// Key identifies the batch: EdgeAnalyticsBatchKey.
	Key       string    `json:"key" gorm:"primaryKey;size:64"`
	EdgeID    string    `json:"edge_id" gorm:"size:255"`
	AppliedAt time.Time `json:"applied_at" gorm:"not null;index"`
}

// EdgeAnalyticsBatchRetention is how long a stored batch is remembered. An
// edge retries a batch for its edge_retention_hours (24 by default) and
// then drops it; twice that covers the default with room to spare.
const EdgeAnalyticsBatchRetention = 48 * time.Hour

// EdgeAnalyticsBatchKey is the key of the batch an edge identifies by its
// ID, the random ID of its process and the pulse's sequence number. The
// parts come from the edge and have no bound; the key is a fixed 64
// characters.
func EdgeAnalyticsBatchKey(edgeID, bootID string, sequence uint64) string {
	sum := sha256.Sum256([]byte(edgeID + "\x00" + bootID + "\x00" + strconv.FormatUint(sequence, 10)))
	return hex.EncodeToString(sum[:])
}

// ClaimEdgeAnalyticsBatch records the batch in tx and reports whether this
// call did: false means a batch with this key was stored before (or is being
// stored by a transaction that then committed), and the caller must store
// nothing. Call it first in the transaction that stores the batch.
func ClaimEdgeAnalyticsBatch(tx *gorm.DB, key, edgeID string, now time.Time) (bool, error) {
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&EdgeAnalyticsBatch{Key: key, EdgeID: edgeID, AppliedAt: now.UTC()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// PruneEdgeAnalyticsBatches deletes the batch records older than
// EdgeAnalyticsBatchRetention and returns how many it deleted. Every replica
// may run it at any time.
func PruneEdgeAnalyticsBatches(db *gorm.DB, now time.Time) (int64, error) {
	res := db.Where("applied_at < ?", now.UTC().Add(-EdgeAnalyticsBatchRetention)).Delete(&EdgeAnalyticsBatch{})
	return res.RowsAffected, res.Error
}
