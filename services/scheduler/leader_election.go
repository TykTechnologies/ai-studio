package scheduler

import (
	"fmt"
	"os"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
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

// TryBecomeLeader attempts to acquire or renew leadership lease
// Returns (isLeader bool, err error)
func (l *LeaderElectionManager) TryBecomeLeader() (bool, error) {
	now := time.Now()
	leaseExpiry := now.Add(l.leaseTTL)

	// Try to get existing lease (ID=1 is singleton)
	var lease models.SchedulerLease
	result := l.db.FirstOrCreate(&lease, models.SchedulerLease{ID: 1})

	if result.Error != nil {
		return false, fmt.Errorf("failed to get lease: %w", result.Error)
	}

	// Check if current lease is expired or we are already the leader
	if lease.ExpiresAt.Before(now) || lease.LeaderID == l.instanceID {
		// Claim/renew leadership
		lease.LeaderID = l.instanceID
		lease.InstanceID = l.instanceID
		lease.ExpiresAt = leaseExpiry
		lease.HeartbeatAt = now

		if err := l.db.Save(&lease).Error; err != nil {
			return false, fmt.Errorf("failed to save lease: %w", err)
		}

		return true, nil
	}

	// Someone else is leader
	return false, nil
}

// IsLeader checks if this instance is currently the leader
func (l *LeaderElectionManager) IsLeader() (bool, error) {
	var lease models.SchedulerLease
	if err := l.db.First(&lease, 1).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}

	// Check if we're the leader and lease hasn't expired
	if lease.LeaderID == l.instanceID && lease.ExpiresAt.After(time.Now()) {
		return true, nil
	}

	return false, nil
}

// ReleaseLease releases leadership (called on graceful shutdown)
func (l *LeaderElectionManager) ReleaseLease() error {
	var lease models.SchedulerLease
	if err := l.db.First(&lease, 1).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil // No lease to release
		}
		return err
	}

	// Only release if we're the leader
	if lease.LeaderID == l.instanceID {
		// Expire the lease immediately
		lease.ExpiresAt = time.Now().Add(-1 * time.Minute)
		return l.db.Save(&lease).Error
	}

	return nil
}

// GetInstanceID returns this instance's unique identifier
func (l *LeaderElectionManager) GetInstanceID() string {
	return l.instanceID
}
