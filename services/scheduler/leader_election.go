package scheduler

import (
	"fmt"
	"os"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// instanceCounter is used to generate unique instance IDs within the same process (for testing)
var instanceCounter int64

// LeaderElectionManager handles leader election using database-based leasing
type LeaderElectionManager struct {
	db         *gorm.DB
	instanceID string
	leaseTTL   time.Duration
}

// NewLeaderElectionManager creates a new leader election manager
func NewLeaderElectionManager(db *gorm.DB) *LeaderElectionManager {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	instanceCounter++
	instanceID := fmt.Sprintf("%s-%d-%d", hostname, os.Getpid(), instanceCounter)

	return &LeaderElectionManager{
		db:         db,
		instanceID: instanceID,
		leaseTTL:   2 * time.Minute, // Lease valid for 2 minutes
	}
}

// TryBecomeLeader acquires the lease if it is free or expired, or renews it
// if this instance holds it. The take-over is one conditional UPDATE, so of
// several replicas trying at once exactly one wins; on Postgres every
// replica compares against the database's clock.
func (l *LeaderElectionManager) TryBecomeLeader() (bool, error) {
	now, err := l.now()
	if err != nil {
		return false, fmt.Errorf("failed to read the database clock: %w", err)
	}

	// The singleton row (ID=1), created expired if missing. Concurrent
	// creators do not collide: the second insert does nothing.
	seed := models.SchedulerLease{ID: 1, InstanceID: "unclaimed", LeaderID: "", ExpiresAt: now.Add(-time.Second), HeartbeatAt: now}
	if err := l.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return false, fmt.Errorf("failed to get lease: %w", err)
	}

	res := l.db.Model(&models.SchedulerLease{}).
		Where("id = ? AND (leader_id = ? OR "+l.before("expires_at")+")", 1, l.instanceID, now).
		Updates(map[string]interface{}{
			"leader_id":    l.instanceID,
			"instance_id":  l.instanceID,
			"expires_at":   now.Add(l.leaseTTL),
			"heartbeat_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return false, fmt.Errorf("failed to save lease: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// IsLeader checks if this instance is currently the leader
func (l *LeaderElectionManager) IsLeader() (bool, error) {
	now, err := l.now()
	if err != nil {
		return false, err
	}
	var n int64
	if err := l.db.Model(&models.SchedulerLease{}).
		Where("id = ? AND leader_id = ? AND NOT "+l.before("expires_at"), 1, l.instanceID, now).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n == 1, nil
}

// ReleaseLease releases leadership (called on graceful shutdown), so
// another replica takes over at its next attempt rather than after the TTL.
func (l *LeaderElectionManager) ReleaseLease() error {
	now, err := l.now()
	if err != nil {
		return err
	}
	return l.db.Model(&models.SchedulerLease{}).
		Where("id = ? AND leader_id = ?", 1, l.instanceID).
		Updates(map[string]interface{}{"expires_at": now.Add(-time.Minute), "updated_at": now}).Error
}

// now is the database's clock on Postgres (replicas' clocks may differ),
// the local clock in UTC otherwise.
func (l *LeaderElectionManager) now() (time.Time, error) {
	if l.db.Dialector.Name() == "postgres" {
		var t time.Time
		if err := l.db.Raw("SELECT now()").Scan(&t).Error; err != nil {
			return time.Time{}, err
		}
		return t.UTC(), nil
	}
	return time.Now().UTC(), nil
}

// before is the condition "column is earlier than the ? argument". SQLite
// compares timestamps as text, and rows written before this change carry
// the local zone, so both sides go through datetime() there.
func (l *LeaderElectionManager) before(column string) string {
	if l.db.Dialector.Name() == "sqlite" {
		return "COALESCE(datetime(" + column + ") < datetime(?), 1)"
	}
	return column + " < ?"
}

// GetInstanceID returns this instance's unique identifier
func (l *LeaderElectionManager) GetInstanceID() string {
	return l.instanceID
}
