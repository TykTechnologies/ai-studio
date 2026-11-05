package grpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// slotEntitlements licenses the default namespace and the first max others
// to be admitted, like the enterprise engine (whose tests cover the real
// counting), so the control server's call sites can be tested in core.
type slotEntitlements struct {
	entitlements.Service
	mu    sync.Mutex
	max   int
	slots map[string]bool
}

func newSlotEntitlements(max int) *slotEntitlements {
	return &slotEntitlements{Service: entitlements.NewCommunityService(), max: max, slots: map[string]bool{}}
}

func (e *slotEntitlements) AdmitNamespace(_ *gorm.DB, ns string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ns = models.CanonicalNamespace(ns)
	if ns == models.DefaultNamespace || e.slots[ns] {
		return true, nil
	}
	if len(e.slots) >= e.max {
		return false, nil
	}
	e.slots[ns] = true
	return true, nil
}

func (e *slotEntitlements) IsNamespaceLicensed(_ *gorm.DB, ns string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	ns = models.CanonicalNamespace(ns)
	return ns == models.DefaultNamespace || e.slots[ns]
}

// passthroughNamespaces is the Enterprise edge management behaviour: the
// requested namespace, canonicalised.
type passthroughNamespaces struct{}

func (passthroughNamespaces) GetNamespaceForEdge(ns string) string {
	return models.CanonicalNamespace(ns)
}
func (passthroughNamespaces) ListNamespaces() ([]string, error) { return nil, nil }
func (passthroughNamespaces) GetNamespaceStats(string) (map[string]interface{}, error) {
	return nil, nil
}

func licensedTestServer(t *testing.T, max int) (*ControlServer, *gorm.DB) {
	t.Helper()
	server, db := setupTestServer(t, nil)
	server.edgeManagementService = passthroughNamespaces{}
	server.SetEntitlements(newSlotEntitlements(max))
	createTestLLMs(db, "")
	return server, db
}

// With one namespace slot: edges in the default namespace and in the first
// namespace get configuration; edges in later namespaces register, are
// marked unlicensed and get no snapshot, no full configuration and no token
// validation, and their namespaces are not recomputed.
func TestNamespaceSlots_UnlicensedNamespaceGetsNoConfig(t *testing.T) {
	server, db := licensedTestServer(t, 1)
	ctx := context.Background()

	register := func(edgeID, ns string) *pb.EdgeRegistrationResponse {
		t.Helper()
		resp, err := server.RegisterEdge(ctx, &pb.EdgeRegistrationRequest{EdgeId: edgeID, EdgeNamespace: ns, Version: "1.0.0"})
		require.NoError(t, err)
		require.True(t, resp.Success, "registration itself succeeds")
		return resp
	}

	for _, ns := range []string{"", "global", "default"} {
		resp := register("edge-default-"+ns, ns)
		assert.NotNil(t, resp.InitialConfig, "default namespace spelled %q uses no slot", ns)
	}
	a := register("edge-a", "team-a")
	assert.NotNil(t, a.InitialConfig)
	b := register("edge-b", "team-b")
	assert.Nil(t, b.InitialConfig)
	assert.Contains(t, b.Message, "not licensed")
	register("edge-c", "team-c")
	register("edge-a2", "team-a")

	assert.Equal(t, "", edgeRow(t, db, "edge-a").LicenseState)
	assert.Equal(t, "", edgeRow(t, db, "edge-a2").LicenseState)
	assert.Equal(t, models.EdgeLicenseStateUnlicensed, edgeRow(t, db, "edge-b").LicenseState)
	assert.Equal(t, models.EdgeLicenseStateUnlicensed, edgeRow(t, db, "edge-c").LicenseState)

	_, err := server.GetFullConfiguration(ctx, &pb.ConfigurationRequest{EdgeId: "edge-b", EdgeNamespace: "team-b"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = server.GetFullConfiguration(ctx, &pb.ConfigurationRequest{EdgeId: "edge-a", EdgeNamespace: "team-a"})
	assert.NoError(t, err)

	// Tokens: refused for the unlicensed edge before any lookup.
	v, err := server.ValidateToken(ctx, &pb.TokenValidationRequest{Token: "whatever", EdgeId: "edge-b", EdgeNamespace: "team-b"})
	require.NoError(t, err)
	assert.False(t, v.Valid)
	assert.Contains(t, v.ErrorMessage, "not licensed")
	v, err = server.ValidateToken(ctx, &pb.TokenValidationRequest{Token: "whatever", EdgeId: "edge-a", EdgeNamespace: "team-a"})
	require.NoError(t, err)
	assert.Equal(t, "Invalid token", v.ErrorMessage, "a licensed edge gets the normal lookup")

	assert.Equal(t, []string{"default", "team-a"}, server.namespacesToRecompute())
}

// Community Edition: the stub licenses every namespace.
func TestNamespaceSlots_CommunityLicensesEverything(t *testing.T) {
	server, _ := setupTestServer(t, nil)
	server.edgeManagementService = passthroughNamespaces{}
	server.SetEntitlements(entitlements.NewCommunityService())
	createTestLLMs(server.db, "")
	resp, err := server.RegisterEdge(context.Background(), &pb.EdgeRegistrationRequest{EdgeId: "e", EdgeNamespace: "team-x", Version: "1.0.0"})
	require.NoError(t, err)
	assert.NotNil(t, resp.InitialConfig)
}

// On the stream: the registration response says the namespace is not
// licensed, and a configuration request from the edge goes unanswered.
func TestNamespaceSlots_StreamUnlicensedNamespace(t *testing.T) {
	server, _ := licensedTestServer(t, 0)

	stream, stop := connect(t, server, "edge-b", "team-b")
	defer stop()
	var reg *pb.EdgeRegistrationResponse
	for _, m := range stream.sent() {
		if r := m.GetRegistrationResponse(); r != nil {
			reg = r
		}
	}
	require.NotNil(t, reg)
	assert.Contains(t, reg.Message, "not licensed")

	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_ConfigRequest{ConfigRequest: &pb.ConfigurationRequest{EdgeId: "edge-b"}}}
	// A licensed edge's request on another stream is answered, so the
	// unlicensed one has been handled by then too.
	licensed, stopLicensed := connect(t, server, "edge-d", "default")
	defer stopLicensed()
	licensed.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_ConfigRequest{ConfigRequest: &pb.ConfigurationRequest{EdgeId: "edge-d"}}}
	require.Eventually(t, func() bool {
		for _, m := range licensed.sent() {
			if m.GetConfiguration() != nil {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Heartbeat{Heartbeat: &pb.HeartbeatRequest{EdgeId: "edge-b"}}}
	require.Eventually(t, func() bool {
		for _, m := range stream.sent() {
			if m.GetHeartbeatResponse() != nil {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "the stream stays up")
	for _, m := range stream.sent() {
		assert.Nil(t, m.GetConfiguration(), "no snapshot for the unlicensed edge")
	}
}
