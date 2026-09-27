package proxy

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// WaitForAnalytics returns once every analysis started with goAnalyze is done.
func TestWaitForAnalytics_WaitsForAnalyzers(t *testing.T) {
	p := &Proxy{}
	var done atomic.Bool
	p.goAnalyze(func() {
		time.Sleep(50 * time.Millisecond)
		done.Store(true)
	})

	assert.NoError(t, p.WaitForAnalytics(context.Background()))
	assert.True(t, done.Load(), "WaitForAnalytics returned before the analysis finished")
}

// A shutdown deadline bounds the wait.
func TestWaitForAnalytics_HonoursContext(t *testing.T) {
	p := &Proxy{}
	release := make(chan struct{})
	defer close(release)
	p.goAnalyze(func() { <-release })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, p.WaitForAnalytics(ctx), context.DeadlineExceeded)
}
