package scheduler

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForLeaderElection(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.SchedulerLease{})
	require.NoError(t, err)

	return db
}

func TestLeaderElectionBasic(t *testing.T) {
	db := setupTestDBForLeaderElection(t)
	lem := NewLeaderElectionManager(db)

	// Should become leader on first attempt
	isLeader, err := lem.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader, "First attempt should acquire leadership")

	// Verify lease in database
	var lease models.SchedulerLease
	err = db.First(&lease, 1).Error
	require.NoError(t, err)

	assert.Equal(t, lem.GetInstanceID(), lease.LeaderID)
	assert.Equal(t, lem.GetInstanceID(), lease.InstanceID)
	assert.True(t, lease.ExpiresAt.After(time.Now()))
	assert.WithinDuration(t, time.Now(), lease.HeartbeatAt, 1*time.Second)
}

func TestLeaderElectionRenewal(t *testing.T) {
	db := setupTestDBForLeaderElection(t)
	lem := NewLeaderElectionManager(db)

	// Become leader
	isLeader, err := lem.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, isLeader)

	// Get initial lease
	var lease1 models.SchedulerLease
	db.First(&lease1, 1)
	initialExpiry := lease1.ExpiresAt
	initialHeartbeat := lease1.HeartbeatAt

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Renew leadership
	isLeader, err = lem.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader, "Should successfully renew leadership")

	// Verify lease was updated
	var lease2 models.SchedulerLease
	db.First(&lease2, 1)

	assert.True(t, lease2.ExpiresAt.After(initialExpiry), "Expiry should be extended")
	assert.True(t, lease2.HeartbeatAt.After(initialHeartbeat), "Heartbeat should be updated")
	assert.Equal(t, lem.GetInstanceID(), lease2.LeaderID)
}

func TestLeaderElectionMultipleInstances(t *testing.T) {
	db := setupTestDBForLeaderElection(t)

	// Create two managers (simulating two AI Studio instances)
	lem1 := NewLeaderElectionManager(db)
	lem2 := NewLeaderElectionManager(db)

	assert.NotEqual(t, lem1.GetInstanceID(), lem2.GetInstanceID(), "Instances should have different IDs")

	// Instance 1 becomes leader
	isLeader1, err := lem1.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader1)

	// Instance 2 tries to become leader (should fail while lease is valid)
	isLeader2, err := lem2.TryBecomeLeader()
	require.NoError(t, err)
	assert.False(t, isLeader2, "Second instance should not become leader while lease is valid")

	// Verify instance 1 is still leader
	var lease models.SchedulerLease
	db.First(&lease, 1)
	assert.Equal(t, lem1.GetInstanceID(), lease.LeaderID)
}

func TestLeaderElectionTakeover(t *testing.T) {
	db := setupTestDBForLeaderElection(t)

	lem1 := NewLeaderElectionManager(db)
	lem2 := NewLeaderElectionManager(db)

	// Instance 1 becomes leader
	isLeader, err := lem1.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, isLeader)

	// Manually expire the lease (simulating instance 1 crash)
	var lease models.SchedulerLease
	db.First(&lease, 1)
	lease.ExpiresAt = time.Now().Add(-1 * time.Minute)
	db.Save(&lease)

	// Instance 2 should now acquire leadership
	isLeader2, err := lem2.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader2, "Second instance should take over after lease expires")

	// Verify new leader
	db.First(&lease, 1)
	assert.Equal(t, lem2.GetInstanceID(), lease.LeaderID)
	assert.True(t, lease.ExpiresAt.After(time.Now()), "New lease should be valid")
}

func TestIsLeader(t *testing.T) {
	db := setupTestDBForLeaderElection(t)
	lem := NewLeaderElectionManager(db)

	// Should not be leader initially
	isLeader, err := lem.IsLeader()
	require.NoError(t, err)
	assert.False(t, isLeader)

	// Become leader
	acquired, err := lem.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, acquired)

	// Should now be leader
	isLeader, err = lem.IsLeader()
	require.NoError(t, err)
	assert.True(t, isLeader)

	// Expire lease
	var lease models.SchedulerLease
	db.First(&lease, 1)
	lease.ExpiresAt = time.Now().Add(-1 * time.Minute)
	db.Save(&lease)

	// Should no longer be leader
	isLeader, err = lem.IsLeader()
	require.NoError(t, err)
	assert.False(t, isLeader, "Should not be leader after lease expires")
}

func TestReleaseLease(t *testing.T) {
	db := setupTestDBForLeaderElection(t)
	lem := NewLeaderElectionManager(db)

	// Become leader
	isLeader, err := lem.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, isLeader)

	// Release lease
	err = lem.ReleaseLease()
	require.NoError(t, err)

	// Verify lease is expired
	var lease models.SchedulerLease
	db.First(&lease, 1)
	assert.True(t, lease.ExpiresAt.Before(time.Now()), "Lease should be expired after release")

	// Another instance should be able to acquire leadership immediately
	lem2 := NewLeaderElectionManager(db)
	isLeader2, err := lem2.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader2, "New instance should acquire leadership after release")
}

func TestReleaseLeaseNotLeader(t *testing.T) {
	db := setupTestDBForLeaderElection(t)

	lem1 := NewLeaderElectionManager(db)
	lem2 := NewLeaderElectionManager(db)

	// Instance 1 becomes leader
	isLeader, err := lem1.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, isLeader)

	// Instance 2 tries to release (should not affect lease)
	err = lem2.ReleaseLease()
	require.NoError(t, err)

	// Verify instance 1 is still leader with valid lease
	var lease models.SchedulerLease
	db.First(&lease, 1)
	assert.Equal(t, lem1.GetInstanceID(), lease.LeaderID)
	assert.True(t, lease.ExpiresAt.After(time.Now()), "Lease should still be valid")
}

func TestLeaderElectionConcurrent(t *testing.T) {
	db := setupTestDBForLeaderElection(t)

	// Create multiple managers
	managers := make([]*LeaderElectionManager, 5)
	for i := 0; i < 5; i++ {
		managers[i] = NewLeaderElectionManager(db)
	}

	// All try to become leader concurrently
	results := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(mgr *LeaderElectionManager) {
			isLeader, err := mgr.TryBecomeLeader()
			require.NoError(t, err)
			results <- isLeader
		}(managers[i])
	}

	// Collect results
	leaderCount := 0
	for i := 0; i < 5; i++ {
		if <-results {
			leaderCount++
		}
	}

	// Only one should be leader
	assert.Equal(t, 1, leaderCount, "Only one instance should become leader")

	// Verify in database
	var lease models.SchedulerLease
	db.First(&lease, 1)
	assert.True(t, lease.ExpiresAt.After(time.Now()))

	// Verify the leader is one of our managers
	foundLeader := false
	for _, mgr := range managers {
		if lease.LeaderID == mgr.GetInstanceID() {
			foundLeader = true
			break
		}
	}
	assert.True(t, foundLeader, "Leader should be one of the test instances")
}
