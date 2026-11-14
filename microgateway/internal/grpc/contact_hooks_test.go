package grpc

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// ackingControl is a configControl that acknowledges each heartbeat, as AI
// Studio does: each acknowledgement is a contact with a control plane.
type ackingControl struct {
	configControl
}

func (a *ackingControl) SubscribeToChanges(stream grpc.BidiStreamingServer[pb.EdgeMessage, pb.ControlMessage]) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if msg.GetHeartbeat() != nil {
			if err := stream.Send(&pb.ControlMessage{Message: &pb.ControlMessage_HeartbeatResponse{
				HeartbeatResponse: &pb.HeartbeatResponse{Acknowledged: true, IsInSync: true},
			}}); err != nil {
				return err
			}
		}
	}
}

func (a *ackingControl) serveAcking(t *testing.T) (string, *grpc.Server) {
	t.Helper()
	srv := grpc.NewServer()
	pb.RegisterConfigurationSyncServiceServer(srv, a)
	addr, _ := serveOn(t, srv)
	return addr, srv
}

// Every part of the edge that asks to hear of contacts and registrations is
// told: adding a hook never replaces one added before.
func TestEdgeClientHooks_AddingNeverReplaces(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{}, "test", "hash", "time")
	var contacts, registrations []string
	client.AddControlContactHook(func(time.Time) { contacts = append(contacts, "first") })
	client.AddControlContactHook(func(time.Time) { contacts = append(contacts, "second") })
	client.AddControlContactHook(nil) // ignored
	client.AddRegisteredHook(func() { registrations = append(registrations, "first") })
	client.AddRegisteredHook(func() { registrations = append(registrations, "second") })

	client.controlContact()
	client.onRegistered.each(func(fn func()) { fn() })
	assert.Equal(t, []string{"first", "second"}, contacts, "in the order they were added")
	assert.Equal(t, []string{"first", "second"}, registrations)
}

// fullSyncEdge is an edge wired as cmd/microgateway wires one with
// EDGE_CREDENTIAL_SYNC=full: contacts recorded first (mustStartEdgeClient),
// then the synced list following the client.
func fullSyncEdge(t *testing.T, db *gorm.DB, mutate func(*config.HubSpokeConfig), addr string) (*SimpleEdgeClient, *services.CredentialSync) {
	t.Helper()
	client := lkgClient(t, db, mutate, addr)
	list, err := services.NewCredentialSync(db, time.Hour)
	require.NoError(t, err)
	client.SyncCredentialsOnContact(list)
	return client, list
}

// With full credential sync on, acknowledged heartbeats still record the
// contact in the edge's database. The sync's hook used to replace the one
// that records it (bead midsommar-clean-4x5y.50), so a full-sync edge kept
// the time of its first registration only.
func TestFullCredentialSync_HeartbeatsStillRecordTheContact(t *testing.T) {
	t.Cleanup(services.SetControlContactWriteInterval(0))
	db := lkgEdgeDB(t, filepath.Join(t.TempDir(), "edge.db"))
	cp := &ackingControl{}
	cp.name = "a"
	cp.snapshot.Store(lkgSnapshot("live"))
	addr, _ := cp.serveAcking(t)

	client, list := fullSyncEdge(t, db, nil, addr)
	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.Nil(t, lkg)
	first, ok, err := services.LastControlContact(db)
	require.NoError(t, err)
	require.True(t, ok)

	// The recorded time has a resolution of a second.
	require.Eventually(t, func() bool {
		at, ok, err := services.LastControlContact(db)
		return err == nil && ok && at.After(first)
	}, 5*time.Second, 50*time.Millisecond, "acknowledged heartbeats record the contact")
	assert.False(t, list.Unavailable(), "the list follows the client too")
	calls := list.SyncCalls()
	assert.EqualValues(t, 1, calls[services.SyncTriggerRegistration], "the registration asked for one sync")
	assert.GreaterOrEqual(t, calls[services.SyncTriggerHeartbeat], uint64(1), "each acknowledged heartbeat asks for one")
	assert.Zero(t, calls[services.SyncTriggerEvent])
}

// A full-sync edge that has run for longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE
// with an unchanged configuration, in contact with a control plane until
// the outage, starts from last-known-good: the age counts from its last
// contact, not from when the configuration was applied.
func TestFullCredentialSync_StartsFromLastKnownGoodAfterALongRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs for a few seconds")
	}
	t.Cleanup(services.SetControlContactWriteInterval(0))
	const maxAge = 3 * time.Second
	withMaxAge := func(hs *config.HubSpokeConfig) { hs.LastKnownGoodMaxAge = maxAge }
	path := filepath.Join(t.TempDir(), "edge.db")
	cp := &ackingControl{}
	cp.name = "a"
	cp.snapshot.Store(lkgSnapshot("stable"))
	addr, srv := cp.serveAcking(t)

	db := lkgEdgeDB(t, path)
	first, _ := fullSyncEdge(t, db, withMaxAge, addr)
	lkg, err := first.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.Nil(t, lkg)
	first.onConfigChange(first.GetCurrentConfiguration()) // cmd/microgateway applies the initial configuration
	require.Equal(t, "stable", storedChecksum(t, db))
	// Longer than the max age, with heartbeats acknowledged throughout and
	// the configuration unchanged.
	time.Sleep(maxAge + 1500*time.Millisecond)
	require.NoError(t, first.Stop())
	require.NoError(t, database.Close(db))

	srv.Stop()
	db2 := lkgEdgeDB(t, path)
	second, _ := fullSyncEdge(t, db2, withMaxAge, addr)
	lkg, err = second.StartWithLastKnownGood(db2)
	require.NoError(t, err, "the last contact is recent: the edge must not refuse")
	require.NotNil(t, lkg)
	assert.Equal(t, "stable", lkg.Checksum)
}

// Hooks may be added while contacts and registrations fire on other
// goroutines (go test -race): a call sees a whole list, the one before an
// add or the one after it.
func TestEdgeClientHooks_AddWhileCalling(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{}, "test", "hash", "time")
	var calls atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				client.controlContact()
				client.onRegistered.each(func(fn func()) { fn() })
			}
		}()
	}
	for i := 0; i < 200; i++ {
		client.AddControlContactHook(func(time.Time) { calls.Add(1) })
		client.AddRegisteredHook(func() { calls.Add(1) })
	}
	close(stop)
	wg.Wait()

	calls.Store(0)
	client.controlContact()
	client.onRegistered.each(func(fn func()) { fn() })
	assert.EqualValues(t, 400, calls.Load(), "every hook added is called")
}

// Stop waits for the stream's goroutines, but a receive loop that never
// ends holds it up for stopWaitTimeout at most, not the whole shutdown.
func TestEdgeClientStop_HungReceiveLoopIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the stop timeout")
	}
	client := NewSimpleEdgeClient(&config.Config{}, "test", "hash", "time")
	client.streamMu.Lock()
	client.streamDone = make(chan struct{}) // never closed
	client.streamMu.Unlock()
	start := time.Now()
	require.NoError(t, client.Stop())
	took := time.Since(start)
	assert.GreaterOrEqual(t, took, stopWaitTimeout)
	assert.Less(t, took, stopWaitTimeout+time.Second)
}
