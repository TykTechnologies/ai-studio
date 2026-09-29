package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEdgeStreamOwnership(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: EdgeStatusRegistered}).Error)
	get := func() EdgeInstance {
		var e EdgeInstance
		require.NoError(t, db.Where("edge_id = ?", "edge-1").First(&e).Error)
		return e
	}

	t.Run("claim records the owner and session", func(t *testing.T) {
		ok, err := ClaimEdgeStream(db, "edge-1", "node-a", "s1")
		require.NoError(t, err)
		assert.True(t, ok)
		e := get()
		assert.Equal(t, "node-a", e.OwnerNodeID)
		assert.Equal(t, "s1", e.StreamSessionID)
		assert.Equal(t, EdgeStatusConnected, e.Status)
		assert.NotNil(t, e.LastHeartbeat)
	})

	t.Run("a reconnect elsewhere survives the old stream closing", func(t *testing.T) {
		_, err := ClaimEdgeStream(db, "edge-1", "node-b", "s2")
		require.NoError(t, err)
		changed, err := ReleaseEdgeStream(db, "edge-1", "s1") // node-a notices its stream ended
		require.NoError(t, err)
		assert.False(t, changed)
		e := get()
		assert.Equal(t, "node-b", e.OwnerNodeID)
		assert.Equal(t, "s2", e.StreamSessionID)
		assert.Equal(t, EdgeStatusConnected, e.Status)
	})

	t.Run("an old stream's heartbeat does not take over", func(t *testing.T) {
		reclaimed, err := TouchEdgeStream(db, "edge-1", "node-a", "s1")
		require.NoError(t, err)
		assert.False(t, reclaimed)
		assert.Equal(t, "node-b", get().OwnerNodeID)
	})

	t.Run("the current owner's heartbeat changes nothing but the time", func(t *testing.T) {
		before := get().LastHeartbeat
		reclaimed, err := TouchEdgeStream(db, "edge-1", "node-b", "s2")
		require.NoError(t, err)
		assert.False(t, reclaimed)
		assert.True(t, get().LastHeartbeat.After(*before) || get().LastHeartbeat.Equal(*before))
	})

	t.Run("the current stream closing clears ownership", func(t *testing.T) {
		changed, err := ReleaseEdgeStream(db, "edge-1", "s2")
		require.NoError(t, err)
		assert.True(t, changed)
		e := get()
		assert.Empty(t, e.OwnerNodeID)
		assert.Empty(t, e.StreamSessionID)
		assert.Equal(t, EdgeStatusDisconnected, e.Status)
	})

	t.Run("a heartbeat on a live stream repairs a cleared row", func(t *testing.T) {
		// The row says disconnected (cleared above), but node-b still
		// receives heartbeats on stream s2: the stream is alive.
		_, err := ClaimEdgeStream(db, "edge-1", "node-b", "s2")
		require.NoError(t, err)
		require.NoError(t, db.Model(&EdgeInstance{}).Where("edge_id = ?", "edge-1").
			Updates(map[string]interface{}{"owner_node_id": "", "stream_session_id": "", "status": EdgeStatusDisconnected}).Error)
		reclaimed, err := TouchEdgeStream(db, "edge-1", "node-b", "s2")
		require.NoError(t, err)
		assert.True(t, reclaimed)
		e := get()
		assert.Equal(t, "node-b", e.OwnerNodeID)
		assert.Equal(t, EdgeStatusConnected, e.Status)
	})

	t.Run("an empty session never matches", func(t *testing.T) {
		changed, err := ReleaseEdgeStream(db, "edge-1", "")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, EdgeStatusConnected, get().Status)
	})

	t.Run("an unknown edge is reported", func(t *testing.T) {
		ok, err := ClaimEdgeStream(db, "no-such-edge", "node-a", "s9")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}
