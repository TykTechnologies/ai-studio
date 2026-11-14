package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	gormlogger "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// The control server's RPCs are not held across a failover
// (pkg/dbhold.Unheld): with no primary an edge's token validation is
// answered at once, so the edge moves to its other control plane, instead of
// waiting out the hold window. Skipped under -short.
func TestFailover_ControlServerRPCFailsFastWithNoPrimary(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	db, err := gorm.Open(postgres.Open(pair.DSN()), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, models.InitModels(db))
	held := dbhold.New(sqlDB, time.Minute, nil, 0)
	db.ConnPool, db.Statement.ConnPool = held, held

	server, err := NewControlServer(&Config{GRPCHost: "localhost", AuthToken: "t", EncryptionKey: "0123456789abcdef0123456789abcdef"}, db)
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	require.NoError(t, pair.StopPrimary(context.Background()))

	started := time.Now()
	resp, err := server.ValidateToken(context.Background(), &pb.TokenValidationRequest{Token: "some-app-key", EdgeId: "edge-a"})
	assert.Less(t, time.Since(started), 10*time.Second, "not held for the window")
	if err == nil {
		assert.False(t, resp.Valid)
	}

	// The same database, held, does wait: the test tells the two apart.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started = time.Now()
	var n int
	scanErr := db.WithContext(ctx).Raw("SELECT 1").Scan(&n).Error
	assert.Error(t, scanErr)
	assert.GreaterOrEqual(t, time.Since(started), 2*time.Second, "a held statement waits")
}
