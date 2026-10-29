//go:build !enterprise

package studio

import (
	"bytes"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/services"
)

const ssoSecretWarning = "TYK_AI_SECRET_KEY is not set: SSO logins are refused until it is set"

// syncBuffer is a log sink safe for Studio's background goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type noHostAuth struct{}

func (noHostAuth) Authenticate(*http.Request) (*services.HostIdentity, error) {
	return nil, errors.New("no identity")
}

// startCapturingLogs starts a Studio with opts and returns what it logged
// during New.
func startCapturingLogs(t *testing.T, opts Options) string {
	t.Helper()
	prev := logger.Log
	t.Cleanup(func() { logger.Use(prev) })
	sink := &syncBuffer{}
	l := zerolog.New(sink).Level(zerolog.DebugLevel)
	opts.Logger = &l

	s, err := New(opts)
	require.NoError(t, err)
	stopStudio(t, s)
	return sink.String()
}

// SSO availability is by edition, not TIB_ENABLED (deprecated), so Studio
// warns at startup whenever the edition has SSO and TYK_AI_SECRET_KEY is
// unset, unless a host does its own authentication (lsj.18).
func TestNewWarnsWhenSSOHasNoSecretKey(t *testing.T) {
	prev := ssoAvailable
	t.Cleanup(func() { ssoAvailable = prev })

	newOpts := func(t *testing.T) Options {
		opts := newTestOptions(t)
		opts.Config.TIBAPISecret = ""
		opts.Config.TIBEnabled = false
		return opts
	}

	t.Run("edition with SSO, key unset", func(t *testing.T) {
		ssoAvailable = func() bool { return true }
		assert.Contains(t, startCapturingLogs(t, newOpts(t)), ssoSecretWarning)
	})

	t.Run("edition without SSO", func(t *testing.T) {
		ssoAvailable = func() bool { return false }
		assert.NotContains(t, startCapturingLogs(t, newOpts(t)), ssoSecretWarning)
	})

	t.Run("host authentication", func(t *testing.T) {
		ssoAvailable = func() bool { return true }
		opts := newOpts(t)
		opts.Auth = noHostAuth{}
		assert.NotContains(t, startCapturingLogs(t, opts), ssoSecretWarning)
	})

	t.Run("key set", func(t *testing.T) {
		ssoAvailable = func() bool { return true }
		opts := newOpts(t)
		opts.Config.TIBAPISecret = "broker-secret"
		assert.NotContains(t, startCapturingLogs(t, opts), ssoSecretWarning)
	})
}
