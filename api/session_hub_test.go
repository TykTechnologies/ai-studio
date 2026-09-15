package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSession struct {
	id      string
	stopped atomic.Int32
}

func (f *fakeSession) ID() string { return f.id }
func (f *fakeSession) Stop()      { f.stopped.Add(1) }

func TestSessionHub_AcquireLoadsOnceAndCountsRefs(t *testing.T) {
	h := NewSessionHub(time.Minute)
	var loads atomic.Int32
	load := func() (HubSession, error) {
		loads.Add(1)
		time.Sleep(20 * time.Millisecond) // widen the race window
		return &fakeSession{id: "s1"}, nil
	}

	var wg sync.WaitGroup
	sessions := make([]HubSession, 5)
	releases := make([]func(), 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, rel, err := h.Acquire("s1", load)
			require.NoError(t, err)
			sessions[i] = s
			releases[i] = rel
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), loads.Load(), "concurrent acquires must share one load")
	for i := 1; i < 5; i++ {
		assert.Same(t, sessions[0], sessions[i], "every caller gets the same session object")
	}
	assert.Equal(t, 5, h.Refs("s1"))

	for _, rel := range releases {
		rel()
		rel() // double release is harmless
	}
	assert.Equal(t, 0, h.Refs("s1"))
	assert.Equal(t, 1, h.Len(), "released sessions stay for the idle TTL")
}

func TestSessionHub_LoadErrorIsNotCached(t *testing.T) {
	h := NewSessionHub(time.Minute)
	calls := 0
	_, _, err := h.Acquire("bad", func() (HubSession, error) {
		calls++
		return nil, errors.New("boom")
	})
	require.Error(t, err)
	assert.Equal(t, 0, h.Len())

	s, rel, err := h.Acquire("bad", func() (HubSession, error) {
		calls++
		return &fakeSession{id: "bad"}, nil
	})
	require.NoError(t, err)
	defer rel()
	assert.Equal(t, 2, calls)
	assert.Equal(t, "bad", s.ID())
}

func TestSessionHub_ReapStopsOnlyIdleUnreferenced(t *testing.T) {
	h := NewSessionHub(time.Minute)
	now := time.Now()
	h.now = func() time.Time { return now }

	held := &fakeSession{id: "held"}
	idle := &fakeSession{id: "idle"}
	fresh := &fakeSession{id: "fresh"}

	_, relHeld := h.Add(held)
	_, relIdle := h.Add(idle)
	relIdle()
	now = now.Add(2 * time.Minute)
	_, relFresh := h.Add(fresh)
	relFresh()

	assert.Equal(t, 1, h.Reap())
	assert.Equal(t, int32(1), idle.stopped.Load())
	assert.Equal(t, int32(0), held.stopped.Load(), "referenced sessions are never reaped")
	assert.Equal(t, int32(0), fresh.stopped.Load(), "recently released sessions survive")
	assert.Equal(t, 2, h.Len())

	relHeld()
	now = now.Add(2 * time.Minute)
	assert.Equal(t, 2, h.Reap())
	assert.Equal(t, 0, h.Len())
}

func TestSessionHub_AddDeduplicatesById(t *testing.T) {
	h := NewSessionHub(time.Minute)
	first := &fakeSession{id: "dup"}
	second := &fakeSession{id: "dup"}

	got1, rel1 := h.Add(first)
	got2, rel2 := h.Add(second)
	defer rel1()
	defer rel2()

	assert.Same(t, first, got1)
	assert.Same(t, first, got2, "the live session wins over a newly built duplicate")
	assert.Equal(t, int32(1), second.stopped.Load(), "the duplicate is stopped, not leaked")
	assert.Equal(t, 2, h.Refs("dup"))
}

func TestSessionHub_RemoveStopsRegardlessOfRefs(t *testing.T) {
	h := NewSessionHub(time.Minute)
	s := &fakeSession{id: "x"}
	_, rel := h.Add(s)
	h.Remove("x")
	assert.Equal(t, int32(1), s.stopped.Load())
	_, ok := h.Get("x")
	assert.False(t, ok)
	rel() // must not panic after removal
}
