package scheduler

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// leaseDBs yields a database shared by several "replicas": a SQLite file,
// and a fresh Postgres schema when DATABASE_URL is set.
func leaseDBs(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lease.db")+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&models.SchedulerLease{}))
		body(t, db)
	})
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		schema := fmt.Sprintf("lease_test_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() {
			admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
			if s, err := admin.DB(); err == nil {
				s.Close()
			}
		})
		u, err := url.Parse(base)
		require.NoError(t, err)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		t.Cleanup(func() {
			if s, err := db.DB(); err == nil {
				s.Close()
			}
		})
		require.NoError(t, db.AutoMigrate(&models.SchedulerLease{}))
		body(t, db)
	})
}

// Replicas starting together, and competing for an expired lease: exactly
// one wins each time. (The lease used to be read and then saved, so two
// replicas could both see it expired and both take it.)
func TestLeaderElection_ExactlyOneWinnerUnderContention(t *testing.T) {
	leaseDBs(t, func(t *testing.T, db *gorm.DB) {
		const replicas = 8
		managers := make([]*LeaderElectionManager, replicas)
		for i := range managers {
			managers[i] = NewLeaderElectionManager(db)
		}
		for round := 0; round < 5; round++ {
			var wg sync.WaitGroup
			var mu sync.Mutex
			winners := 0
			start := make(chan struct{})
			for _, m := range managers {
				wg.Add(1)
				go func(m *LeaderElectionManager) {
					defer wg.Done()
					<-start
					ok, err := m.TryBecomeLeader()
					require.NoError(t, err)
					if ok {
						mu.Lock()
						winners++
						mu.Unlock()
					}
				}(m)
			}
			close(start)
			wg.Wait()
			assert.Equal(t, 1, winners, "round %d", round)

			leaders := 0
			for _, m := range managers {
				ok, err := m.IsLeader()
				require.NoError(t, err)
				if ok {
					leaders++
				}
			}
			assert.Equal(t, 1, leaders, "round %d: exactly one replica believes it leads", round)

			// Expire the lease for the next round.
			require.NoError(t, db.Model(&models.SchedulerLease{}).Where("id = 1").
				Update("expires_at", time.Now().UTC().Add(-time.Minute)).Error)
		}
	})
}

// The holder renews; others cannot take a live lease; a released lease is
// taken at once by another replica.
func TestLeaderElection_RenewAndHandOver(t *testing.T) {
	leaseDBs(t, func(t *testing.T, db *gorm.DB) {
		a, b := NewLeaderElectionManager(db), NewLeaderElectionManager(db)
		ok, err := a.TryBecomeLeader()
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = b.TryBecomeLeader()
		require.NoError(t, err)
		assert.False(t, ok, "a live lease is not taken")
		ok, err = a.TryBecomeLeader()
		require.NoError(t, err)
		assert.True(t, ok, "the holder renews")

		require.NoError(t, b.ReleaseLease(), "releasing a lease one does not hold is a no-op")
		ok, _ = a.IsLeader()
		assert.True(t, ok)

		require.NoError(t, a.ReleaseLease())
		ok, _ = a.IsLeader()
		assert.False(t, ok)
		ok, err = b.TryBecomeLeader()
		require.NoError(t, err)
		assert.True(t, ok, "released lease taken at once")
	})
}

// A lease row written by an older version in the local zone (SQLite
// compares timestamps as text) is still read correctly.
func TestLeaderElection_LocalZoneLeaseRows(t *testing.T) {
	leaseDBs(t, func(t *testing.T, db *gorm.DB) {
		east := time.FixedZone("east", 14*3600)
		live := time.Now().Add(time.Minute).In(time.FixedZone("west", -12*3600)) // text sorts early
		require.NoError(t, db.Create(&models.SchedulerLease{ID: 1, InstanceID: "old", LeaderID: "old", ExpiresAt: live, HeartbeatAt: time.Now().In(east)}).Error)
		m := NewLeaderElectionManager(db)
		ok, err := m.TryBecomeLeader()
		require.NoError(t, err)
		assert.False(t, ok, "the old holder's lease is still live")

		expired := time.Now().Add(-time.Minute).In(east) // text sorts late
		require.NoError(t, db.Model(&models.SchedulerLease{}).Where("id = 1").Update("expires_at", expired).Error)
		ok, err = m.TryBecomeLeader()
		require.NoError(t, err)
		assert.True(t, ok, "an expired lease is taken over")
	})
}
