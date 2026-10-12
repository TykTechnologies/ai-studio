//go:build !enterprise

package studio

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stop cancels the legacy secret re-encryption and waits for it: it logs
// through the process-wide logger, which the next Studio or control plane
// in the process replaces (a data race when it outlived Stop).
func TestStopWaitsForTheSecretReencryption(t *testing.T) {
	var finished atomic.Bool
	release := make(chan struct{})
	reencryptSecrets = func(ctx context.Context, _ *gorm.DB) {
		close(release)
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond) // a batch still in flight
		finished.Store(true)
	}
	t.Cleanup(func() { reencryptSecrets = reencryptLegacySecrets })

	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	<-release
	assert.False(t, finished.Load(), "it runs until Stop")

	stopStudio(t, s)
	assert.True(t, finished.Load(), "Stop returned before the re-encryption ended")
}
