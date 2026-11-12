package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	gormlogger "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// The lease is not held across a failover (pkg/dbhold.Unheld): with no
// primary, a renewal or takeover fails at once instead of waiting out the
// hold window, which could outlast the lease's term. Skipped under -short.
func TestFailover_LeaseFailsFastWithNoPrimary(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	db, err := gorm.Open(postgres.Open(pair.DSN()), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.ClusterLease{}, &models.ClusterNode{}))
	held := dbhold.New(sqlDB, time.Minute, nil, 0)
	db.ConnPool, db.Statement.ConnPool = held, held

	l := NewLeadership(db, "unheld-test", "node-a", LeadershipOptions{})
	require.NoError(t, pair.StopPrimary(context.Background()))

	started := time.Now()
	_, _, err = l.acquire()
	assert.Error(t, err)
	assert.Less(t, time.Since(started), 10*time.Second, "not held for the window")
}
