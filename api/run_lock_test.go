package api

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A run request that arrives while the previous turn's handler is still
// unlocking must wait for it rather than be refused.
func TestLockRunWithin(t *testing.T) {
	var mu sync.Mutex
	mu.Lock()
	go func() {
		time.Sleep(50 * time.Millisecond)
		mu.Unlock()
	}()
	assert.True(t, lockRunWithin(context.Background(), mu.TryLock, time.Second))
	mu.Unlock()

	mu.Lock()
	assert.False(t, lockRunWithin(context.Background(), mu.TryLock, 40*time.Millisecond), "gives up after the timeout")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, lockRunWithin(ctx, mu.TryLock, time.Second), "a cancelled request stops waiting")
	mu.Unlock()
}
