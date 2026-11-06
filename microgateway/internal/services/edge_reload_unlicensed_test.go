package services

import (
	"sync"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// unlicensedThenLicensedClient answers GetFullConfiguration like a hub whose
// namespace has no licensed slot (FailedPrecondition, no configuration)
// until licensed is set, then with a snapshot.
type unlicensedThenLicensedClient struct {
	mu       sync.Mutex
	licensed bool
	config   *pb.ConfigurationSnapshot
}

func (c *unlicensedThenLicensedClient) RequestFullSync() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.licensed {
		return status.Error(codes.FailedPrecondition, `namespace "team-b" is not licensed`)
	}
	c.config = &pb.ConfigurationSnapshot{Version: "v1", SnapshotTime: timestamppb.Now()}
	return nil
}

func (c *unlicensedThenLicensedClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config
}

// An edge whose namespace waited for a slot has no configuration and saw
// its pulls refused; once the hub admits the namespace and pushes, the
// edge applies the snapshot it pulls, with no restart.
func TestReloadAppliesConfigurationAfterNamespaceIsLicensed(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	client := &unlicensedThenLicensedClient{}
	var mu sync.Mutex
	var phases []pb.ReloadPhase
	h := NewEdgeReloadHandler(client, NewEdgeSyncService(db, "team-b"), db, "edge-b", func(r *pb.ConfigurationReloadResponse) {
		mu.Lock()
		defer mu.Unlock()
		phases = append(phases, r.Phase)
	}, nil)
	last := func() pb.ReloadPhase {
		mu.Lock()
		defer mu.Unlock()
		return phases[len(phases)-1]
	}

	h.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-1"})
	assert.Equal(t, pb.ReloadPhase_FAILED, last(), "refused while unlicensed")
	assert.Nil(t, client.GetCurrentConfiguration())

	client.mu.Lock()
	client.licensed = true
	client.mu.Unlock()
	h.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-2"})
	require.Equal(t, pb.ReloadPhase_READY, last())
	assert.Equal(t, "v1", h.currentVersion, "the pulled snapshot is applied")
}
