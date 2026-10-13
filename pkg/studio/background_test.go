//go:build !enterprise

package studio

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/services"
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

// The same for the notification body backfill: it queries through gorm,
// whose logger reads the process-wide log level, so outliving Stop raced
// the next control plane's logger.Use (seen in the pkg/studio Postgres job).
func TestStopWaitsForTheNotificationBackfill(t *testing.T) {
	var finished atomic.Bool
	release := make(chan struct{})
	orig := backfillNotificationBodies
	backfillNotificationBodies = func(ctx context.Context, _ *services.NotificationService) (int64, error) {
		close(release)
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond) // a page still in flight
		finished.Store(true)
		return 0, ctx.Err()
	}
	t.Cleanup(func() { backfillNotificationBodies = orig })

	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	<-release
	assert.False(t, finished.Load(), "it runs until Stop")

	stopStudio(t, s)
	assert.True(t, finished.Load(), "Stop returned before the backfill ended")
}
