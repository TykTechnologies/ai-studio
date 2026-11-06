package grpc

import (
	"bytes"
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newControlModeTestServer(t *testing.T) (*ControlServer, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	s := NewControlServer(&config.Config{HubSpoke: config.HubSpokeConfig{Mode: "control"}}, db)
	t.Cleanup(s.Stop)
	return s, db
}

func TestControlServerLogsDeprecationWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })

	newControlModeTestServer(t)

	assert.Contains(t, buf.String(), `"level":"warn"`)
	assert.Contains(t, buf.String(), ControlModeDeprecationMessage)
}

func TestControlModeNamespaces(t *testing.T) {
	s, db := newControlModeTestServer(t)
	ctx := context.Background()

	register := func(edgeID, ns string) error {
		_, err := s.RegisterEdge(ctx, &pb.EdgeRegistrationRequest{EdgeId: edgeID, EdgeNamespace: ns, Version: "1.0.0",
			Health: &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY, Timestamp: timestamppb.Now()}})
		return err
	}

	// The default namespace, in each spelling, is always served.
	for i, ns := range []string{"", "global", "default"} {
		require.NoError(t, register("default-edge-"+string(rune('a'+i)), ns), ns)
		_, err := s.GetFullConfiguration(ctx, &pb.ConfigurationRequest{EdgeId: "x", EdgeNamespace: ns})
		require.NoError(t, err, ns)
	}

	err := register("team-edge", "team-a")
	_, cfgErr := s.GetFullConfiguration(ctx, &pb.ConfigurationRequest{EdgeId: "team-edge", EdgeNamespace: "team-a"})
	_, tokErr := s.ValidateToken(ctx, &pb.TokenValidationRequest{Token: "tok-123456789", EdgeId: "team-edge", EdgeNamespace: "team-a"})
	var stored int64
	require.NoError(t, db.Model(&database.EdgeInstance{}).Where("edge_id = ?", "team-edge").Count(&stored).Error)

	if !controlModeDefaultNamespaceOnly {
		// Community: unchanged, every namespace is served.
		require.NoError(t, err)
		require.NoError(t, cfgErr)
		assert.Equal(t, int64(1), stored)
		return
	}

	// Enterprise: refused with a pointer to AI Studio, no record, no config.
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "register this edge against AI Studio")
	assert.Equal(t, int64(0), stored)
	s.edgeMutex.RLock()
	_, inMemory := s.edgeInstances["team-edge"]
	s.edgeMutex.RUnlock()
	assert.False(t, inMemory)
	assert.Equal(t, codes.FailedPrecondition, status.Code(cfgErr))
	assert.Equal(t, codes.FailedPrecondition, status.Code(tokErr))
}
