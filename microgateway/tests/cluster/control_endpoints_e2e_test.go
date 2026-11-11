package cluster

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// These tests give an edge two control planes (CONTROL_ENDPOINTS), one per
// site, each behind a forwarder of its own, and cut a site. See
// features/ActiveActive.md, Phase 2.

// twoSites starts two replicas, a forwarder in front of each, and an edge
// that lists site A first.
func twoSites(t *testing.T, sdb studioDB, failback time.Duration) (a, b *replica, fwdA, fwdB *forwarder, e *edge) {
	t.Helper()
	a = startReplica(t, sdb, "node-a")
	b = startReplica(t, sdb, "node-b")
	fwdA = newForwarder(t, a.addr)
	fwdB = newForwarder(t, b.addr)
	e = startEdgeWith(t, "edge-1", "", fwdA, edgeOptions{more: []*forwarder{fwdB}, failback: failback})
	waitOwner(t, b.db, "edge-1", "node-a")
	return a, b, fwdA, fwdB, e
}

// addKey creates an App with a key in Studio and returns the key.
func addKey(t *testing.T, db *gorm.DB, name, key string) {
	t.Helper()
	cred := &models.Credential{KeyID: name + "-key-id", Secret: key, Active: true}
	require.NoError(t, db.Create(cred).Error)
	require.NoError(t, db.Create(&models.App{Name: name, IsActive: true, CredentialID: cred.ID, Namespace: ""}).Error)
}

// Control plane A is cut while a push is in flight: the edge connects to B
// without waiting for a backoff, B delivers the push again, and it ends
// succeeded there.
func TestE2E_ControlPlaneCutMidPush_EdgeFinishesOnTheOther(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a, b, fwdA, _, e := twoSites(t, sdb, 0)

		addLLM(t, a.db, "failover-llm")
		op := push(t, b, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		// The edge acknowledged and is pulling (the handler waits two
		// seconds for the snapshot before it answers).
		require.Eventually(t, func() bool {
			st, err := a.pushes.Status(context.Background(), op)
			return err == nil && len(st.Commands) == 1 && st.Commands[0].Phase == "PULL_STARTED"
		}, 20*time.Second, 20*time.Millisecond)

		cut := time.Now()
		fwdA.fail()

		waitOwner(t, b.db, "edge-1", "node-b")
		// The edge's reconnect interval is 100 ms. It went to B at once, so
		// it is there well before a backoff pass (100 ms, 200 ms, ...) and
		// a registration could have taken it.
		moved := time.Since(cut)
		t.Logf("edge on node-b %v after the cut", moved.Round(time.Millisecond))
		assert.Less(t, moved, 5*time.Second)
		assert.Equal(t, fwdA.addr(), e.client.ControlLink().Endpoints[0])
		link := e.client.ControlLink()
		assert.True(t, link.Connected)
		assert.Equal(t, e.fwdMore[0].addr(), link.Endpoint, "the edge reports the endpoint it is on")

		st := waitPush(t, b, op, models.PushOperationSucceeded, 40*time.Second)
		cmd := st.Commands[0]
		assert.Equal(t, 2, cmd.Attempts, "history: %+v", cmd.History)
		assert.Equal(t, "node-b", cmd.History[len(cmd.History)-1].Node)
		assert.True(t, e.hasLLM("failover-llm"), "the configuration is in the edge's database")
	})
}

// A key created through control plane A just before A is lost validates on
// the edge through B: both write to the one database.
func TestE2E_KeyCreatedBeforeTheCutValidatesThroughTheOther(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a, b, fwdA, fwdB, e := twoSites(t, sdb, 0)

		const key = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
		addKey(t, a.db, "failover-app", key)
		fwdA.fail()
		a.crash()

		// The edge has never seen the key; it asks whichever control plane
		// it is on.
		waitOwner(t, b.db, "edge-1", "node-b")
		resp, err := e.client.ValidateTokenOnDemand(key)
		require.NoError(t, err)
		require.True(t, resp.Valid, resp.ErrorMessage)
		assert.Equal(t, "failover-app", resp.AppName)
		assert.Equal(t, fwdB.addr(), e.client.ControlLink().Endpoint)

		// And a key created on B while A is gone.
		const second = "0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d"
		addKey(t, b.db, "after-failover-app", second)
		resp, err = e.client.ValidateTokenOnDemand(second)
		require.NoError(t, err)
		require.True(t, resp.Valid, resp.ErrorMessage)
	})
}

// With EDGE_CONTROL_FAILBACK_INTERVAL set the edge returns to A once A is
// restored; a push made afterwards reaches it there.
func TestE2E_FailbackReturnsTheEdgeToItsFirstControlPlane(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a, b, fwdA, _, e := twoSites(t, sdb, 300*time.Millisecond)

		fwdA.fail()
		waitOwner(t, b.db, "edge-1", "node-b")

		fwdA.setUp()
		waitOwner(t, b.db, "edge-1", "node-a")
		link := e.client.ControlLink()
		assert.Equal(t, fwdA.addr(), link.Endpoint)

		addLLM(t, b.db, "after-failback-llm")
		op := push(t, b, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		st := waitPush(t, a, op, models.PushOperationSucceeded, 30*time.Second)
		assert.Equal(t, "node-a", st.Commands[0].History[len(st.Commands[0].History)-1].Node)
		assert.True(t, e.hasLLM("after-failback-llm"))

		// It stays on A.
		time.Sleep(time.Second)
		owner, status := streamOwner(t, a.db, "edge-1")
		assert.Equal(t, "node-a", owner)
		assert.Equal(t, models.EdgeStatusConnected, status)
	})
}

// Without the failback interval the edge stays on B after A is restored,
// until its connection to B ends.
func TestE2E_NoFailback_EdgeStaysUntilItsConnectionEnds(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		_, b, fwdA, fwdB, e := twoSites(t, sdb, 0)

		fwdA.fail()
		waitOwner(t, b.db, "edge-1", "node-b")

		fwdA.setUp()
		time.Sleep(1500 * time.Millisecond) // many reconnect intervals (100 ms)
		owner, status := streamOwner(t, b.db, "edge-1")
		assert.Equal(t, "node-b", owner, "the edge left B although its connection there was fine")
		assert.Equal(t, models.EdgeStatusConnected, status)
		assert.Equal(t, fwdB.addr(), e.client.ControlLink().Endpoint)

		// The connection to B ends (as GRPC_MAX_CONNECTION_AGE would end
		// it): the next endpoint in the list after B is A.
		fwdB.cut()
		waitOwner(t, b.db, "edge-1", "node-a")
	})
}

// Both control planes are away for a while: the edge keeps trying both and
// takes whichever returns, here the second.
func TestE2E_BothControlPlanesCut_EdgeTakesTheOneThatReturns(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		_, b, fwdA, fwdB, e := twoSites(t, sdb, 0)

		fwdB.setDown()
		fwdA.fail()
		time.Sleep(time.Second) // several passes over the list fail
		assert.False(t, e.client.ControlLink().Connected)

		fwdB.setUp()
		waitOwner(t, b.db, "edge-1", "node-b")

		addLLM(t, b.db, "after-outage-llm")
		op := push(t, b, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		waitPush(t, b, op, models.PushOperationSucceeded, 30*time.Second)
		assert.True(t, e.hasLLM("after-outage-llm"))
	})
}

// The edge fails back to control plane A while a push is in flight on B and
// while requests are validating keys. The failback's probe registers the
// edge on A although its stream is still open on B: that registration
// leaves the stream's ownership alone (RegisterEdge does not write the
// owner or the stream session), so B goes on delivering until the edge
// closes that stream; B then hands the push back and A delivers it again
// when the edge's stream opens there. No validation fails: calls use the
// connection to B until the one to A is registered, and those in flight
// finish on B's connection.
func TestE2E_FailbackWithPushAndValidationsInFlight(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a, b, fwdA, _, e := twoSites(t, sdb, 300*time.Millisecond)
		const key = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
		addKey(t, a.db, "failback-app", key)

		fwdA.fail()
		waitOwner(t, b.db, "edge-1", "node-b")

		// Requests validate keys throughout.
		stop := make(chan struct{})
		var validations, failures atomic.Int64
		var firstFailure atomic.Value
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					resp, err := e.client.ValidateTokenOnDemand(key)
					validations.Add(1)
					if err != nil {
						failures.Add(1)
						firstFailure.CompareAndSwap(nil, err.Error())
					} else if !resp.Valid {
						failures.Add(1)
						firstFailure.CompareAndSwap(nil, "rejected: "+resp.ErrorMessage)
					}
				}
			}()
		}

		// A push is in flight on B: acknowledged, the edge is pulling.
		addLLM(t, b.db, "during-failback-llm")
		op := push(t, a, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		require.Eventually(t, func() bool {
			st, err := b.pushes.Status(context.Background(), op)
			return err == nil && len(st.Commands) == 1 && st.Commands[0].Phase == "PULL_STARTED"
		}, 20*time.Second, 20*time.Millisecond)
		before := validations.Load()

		// A returns; the edge fails back within its 300 ms interval.
		fwdA.setUp()
		waitOwner(t, b.db, "edge-1", "node-a")
		require.Eventually(t, func() bool { return validations.Load() > before+50 }, 20*time.Second, 10*time.Millisecond,
			"validations kept running across the failback")

		st := waitPush(t, a, op, models.PushOperationSucceeded, 40*time.Second)
		assert.True(t, e.hasLLM("during-failback-llm"), "the pushed configuration is on the edge")
		t.Logf("push: %d attempt(s), history %+v", st.Commands[0].Attempts, st.Commands[0].History)

		close(stop)
		wg.Wait()
		assert.Zero(t, failures.Load(), "of %d validations across the failback; first failure: %v", validations.Load(), firstFailure.Load())

		// The hub's view after the move: one owner, connected, and it stays.
		time.Sleep(time.Second)
		owner, status := streamOwner(t, a.db, "edge-1")
		assert.Equal(t, "node-a", owner)
		assert.Equal(t, models.EdgeStatusConnected, status, "the probe's registration did not leave the edge as registered-only")
		assert.Equal(t, fwdA.addr(), e.client.ControlLink().Endpoint)
	})
}
