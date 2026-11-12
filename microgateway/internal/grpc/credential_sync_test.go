package grpc

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// syncControl is a control plane that answers SyncCredentials with what the
// test scripted: pages, or an error, and "register first" until the edge
// has registered again.
type syncControl struct {
	*siteControl
	mu       sync.Mutex
	pages    []*pb.CredentialSyncPage
	err      error
	requests []*pb.CredentialSyncRequest
	// forgetUntil makes the control plane answer "register first" while
	// the edge has registered fewer times than this.
	forgetUntil atomic.Int32
}

func (s *syncControl) SyncCredentials(req *pb.CredentialSyncRequest, stream grpclib.ServerStreamingServer[pb.CredentialSyncPage]) error {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	pages, err := s.pages, s.err
	s.mu.Unlock()
	if s.siteControl.registrations.Load() < s.forgetUntil.Load() {
		return status.Errorf(codes.FailedPrecondition, "edge %q is not registered with this control plane: register first", req.GetEdgeId())
	}
	if err != nil {
		return err
	}
	for _, p := range pages {
		if err := stream.Send(p); err != nil {
			return err
		}
	}
	return nil
}

func (s *syncControl) lastRequest() *pb.CredentialSyncRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return nil
	}
	return s.requests[len(s.requests)-1]
}

func startSyncEdge(t *testing.T) (*syncControl, *SimpleEdgeClient) {
	t.Helper()
	ctl := &syncControl{siteControl: newSiteControl("site-a", nil)}
	srv := grpclib.NewServer()
	pb.RegisterConfigurationSyncServiceServer(srv, ctl)
	addr, _ := serveOn(t, srv)
	client := endpointsClient(t, 20*time.Millisecond, nil, addr)
	require.NoError(t, client.Start())
	waitLink(t, client, addr)
	return ctl, client
}

func edgeListDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() { database.Close(db) })
	return db
}

var testHash = "v1:" + strings.Repeat("ab", 32)

// The edge asks for its own ID, namespace, watermark and hash version, and
// the pages arrive converted, in order, ending with the last.
func TestCredentialSyncSource_StreamsPages(t *testing.T) {
	ctl, client := startSyncEdge(t)
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	ctl.pages = []*pb.CredentialSyncPage{
		{Full: true, HashVersion: "v1", Credentials: []*pb.SyncedCredential{{CredentialId: 1, SecretHash: testHash, AppId: 7, Active: true, UpdatedAt: timestamppb.New(at)}},
			Apps: []*pb.AppConfig{{Id: 7, Name: "app"}}},
		{Full: true, Last: true, Watermark: 42, HashVersion: "v1", Credentials: []*pb.SyncedCredential{{CredentialId: 2, Deleted: true}}},
	}
	var got []*services.CredentialSyncPage
	require.NoError(t, client.SyncCredentials(context.Background(), 9, func(p *services.CredentialSyncPage) error {
		got = append(got, p)
		return nil
	}))
	req := ctl.lastRequest()
	require.NotNil(t, req)
	assert.Equal(t, "endpoints-edge", req.EdgeId)
	assert.Equal(t, "test", req.EdgeNamespace)
	assert.EqualValues(t, 9, req.Since)
	assert.Equal(t, "v1", req.HashVersion)

	require.Len(t, got, 2)
	assert.Equal(t, services.SyncedCredentialUpdate{CredentialID: 1, SecretHash: testHash, AppID: 7, Active: true, UpdatedAt: at}, got[0].Credentials[0])
	assert.EqualValues(t, 7, got[0].Apps[0].Id)
	assert.True(t, got[0].Full)
	assert.False(t, got[0].Last)
	assert.True(t, got[1].Last)
	assert.EqualValues(t, 42, got[1].Watermark)
	assert.Equal(t, services.SyncedCredentialUpdate{CredentialID: 2, Deleted: true}, got[1].Credentials[0])
}

// What the control plane answers maps onto what the sync does with it.
func TestCredentialSyncSource_MapsTheControlPlanesAnswers(t *testing.T) {
	ctl, client := startSyncEdge(t)
	for _, tc := range []struct {
		err  error
		want error
	}{
		{status.Error(codes.Unimplemented, "credential sync is not available on this control plane"), services.ErrCredentialSyncUnavailable},
		{status.Errorf(codes.FailedPrecondition, "credential hash version %q is not supported (this control plane sends %q)", "v1", "v2"), services.ErrCredentialHashVersion},
		{status.Error(codes.Unavailable, "credential sync: the database is not available"), nil},
	} {
		ctl.mu.Lock()
		ctl.err = tc.err
		ctl.mu.Unlock()
		err := client.SyncCredentials(context.Background(), 0, func(*services.CredentialSyncPage) error { return nil })
		require.Error(t, err)
		if tc.want != nil {
			assert.ErrorIs(t, err, tc.want)
		} else {
			assert.False(t, errors.Is(err, services.ErrCredentialSyncUnavailable), "an unavailable database is not a control plane without the sync")
			assert.Contains(t, err.Error(), "database is not available")
		}
	}

	ctl.mu.Lock()
	ctl.err = nil
	ctl.mu.Unlock()
	ctl.forgetUntil.Store(ctl.siteControl.registrations.Load() + 1)
	err := client.SyncCredentials(context.Background(), 0, func(*services.CredentialSyncPage) error { return nil })
	assert.ErrorIs(t, err, services.ErrCredentialSyncNotRegistered)
	assert.False(t, errors.Is(err, services.ErrCredentialSyncUnavailable), "not the on-demand fallback")
}

// A control plane that does not know the edge ("register first") gets a
// new registration, and the sync that follows it completes: the edge does
// not fall back to validating on demand.
func TestCredentialSyncSource_RegistersAgainWhenNotKnown(t *testing.T) {
	ctl, client := startSyncEdge(t)
	ctl.pages = []*pb.CredentialSyncPage{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
		Credentials: []*pb.SyncedCredential{{CredentialId: 1, SecretHash: testHash, AppId: 7, Active: true}},
		Apps:        []*pb.AppConfig{{Id: 7, Name: "app", IsActive: true}}}}
	before := ctl.siteControl.registrations.Load()
	ctl.forgetUntil.Store(before + 1)

	list, err := services.NewCredentialSync(edgeListDB(t), time.Hour)
	require.NoError(t, err)
	client.SetRegisteredHook(list.Reset)
	client.SetControlContactHook(func(time.Time) { list.Kick() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go list.Run(ctx, client, time.Minute)
	list.Kick()

	require.Eventually(t, func() bool { return list.Count() == 1 }, 10*time.Second, 20*time.Millisecond, "synced after registering again")
	assert.Greater(t, ctl.siteControl.registrations.Load(), before, "registered again")
	assert.False(t, list.LastSuccess().IsZero())
}

// serveOn serves srv on a free loopback port.
func serveOn(t *testing.T, srv *grpclib.Server) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), srv.Stop
}
