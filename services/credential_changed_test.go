package services

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
)

const edgeKey = "12345678901234567890123456789012"

// changeSink collects credential.changed events from a bus.
type changeSink struct {
	mu     sync.Mutex
	events []eventbridge.Event
}

func (c *changeSink) handle(e eventbridge.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

// take waits for n events and returns their payloads, and empties the sink.
func (c *changeSink) take(t *testing.T, n int) []eventbridge.CredentialChanged {
	t.Helper()
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.events) >= n
	}, 2*time.Second, 5*time.Millisecond, "%d credential.changed events", n)
	time.Sleep(20 * time.Millisecond) // and no more
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Len(t, c.events, n)
	var out []eventbridge.CredentialChanged
	for _, e := range c.events {
		assert.Equal(t, eventbridge.DirDown, e.Dir, "for the edges")
		change, err := eventbridge.ParseCredentialChanged(e.Payload)
		require.NoError(t, err)
		out = append(out, change)
	}
	c.events = nil
	return out
}

func credentialChangedService(t *testing.T) (*Service, *changeSink) {
	t.Helper()
	svc := NewService(setupTestDBForCredentials(t))
	bus := eventbridge.NewBus()
	svc.EventBus = bus
	sink := &changeSink{}
	sub := bus.Subscribe(eventbridge.CredentialChangedTopic, sink.handle)
	t.Cleanup(func() { bus.Unsubscribe(sub) })
	SetEdgeCredentialHasher(edgeKey)
	t.Cleanup(func() { SetEdgeCredentialHasher("") })
	return svc, sink
}

func edgeHash(t *testing.T, secret string) string {
	t.Helper()
	h, err := credhash.New(edgeKey)
	require.NoError(t, err)
	return h.Hash(secret)
}

// Every change to an App key tells the edges, naming the key by the hash
// they hold for it (never the key): created, activated, deactivated,
// regenerated (the old key as deleted, then the new one) and deleted.
func TestCredentialChanged_RaisedForEveryKeyChange(t *testing.T) {
	svc, sink := credentialChangedService(t)

	cred, err := svc.CreateCredential()
	require.NoError(t, err)
	got := sink.take(t, 1)
	assert.Equal(t, eventbridge.CredentialChanged{CredentialID: cred.ID, Hash: edgeHash(t, cred.Secret), Active: false}, got[0])
	assert.NotContains(t, got[0].Hash, cred.Secret)

	require.NoError(t, svc.ActivateCredential(cred.ID))
	assert.True(t, sink.take(t, 1)[0].Active)
	require.NoError(t, svc.DeactivateCredential(cred.ID))
	assert.False(t, sink.take(t, 1)[0].Active)

	old := cred.Secret
	cred, err = svc.GetCredentialByID(cred.ID)
	require.NoError(t, err)
	cred.Secret = "a-regenerated-secret-0123456789abcdef"
	require.NoError(t, svc.UpdateCredential(cred))
	got = sink.take(t, 2)
	assert.Equal(t, edgeHash(t, old), got[0].Hash, "the old key first")
	assert.True(t, got[0].Deleted)
	assert.Equal(t, edgeHash(t, cred.Secret), got[1].Hash)
	assert.False(t, got[1].Deleted)

	require.NoError(t, svc.DeleteCredential(cred.ID))
	got = sink.take(t, 1)
	assert.Equal(t, edgeHash(t, cred.Secret), got[0].Hash, "hashed before the row went")
	assert.True(t, got[0].Deleted)
	assert.False(t, got[0].Active)
}

// An App's activation and its key's activation through the App tell the
// edges, with the App and its namespace.
func TestCredentialChanged_RaisedForAppChanges(t *testing.T) {
	svc, sink := credentialChangedService(t)
	cred, err := svc.CreateCredential()
	require.NoError(t, err)
	app := &models.App{Name: "events-app", IsActive: true, CredentialID: cred.ID, Namespace: "eu"}
	require.NoError(t, svc.DB.Create(app).Error)
	sink.take(t, 1)

	require.NoError(t, svc.ActivateAppCredential(app.ID))
	got := sink.take(t, 1)
	assert.Equal(t, eventbridge.CredentialChanged{CredentialID: cred.ID, Hash: edgeHash(t, cred.Secret), AppID: app.ID, Namespace: "eu", Active: true}, got[0])

	_, err = svc.SetAppActive(app.ID, false, 0)
	require.NoError(t, err)
	got = sink.take(t, 1)
	assert.False(t, got[0].Active, "an inactive App's key")
	assert.Equal(t, app.ID, got[0].AppID)

	require.NoError(t, svc.DeactivateAppCredential(app.ID))
	assert.False(t, sink.take(t, 1)[0].Active)
}

// No event without a usable MICROGATEWAY_ENCRYPTION_KEY (no hash edges
// could match), and none for a secret this replica cannot read.
func TestCredentialChanged_NotRaisedWithoutAHash(t *testing.T) {
	svc, sink := credentialChangedService(t)
	SetEdgeCredentialHasher("short")
	_, err := svc.CreateCredential()
	require.NoError(t, err)

	SetEdgeCredentialHasher(edgeKey)
	_, ok := svc.credentialChange(&models.Credential{ID: 9, Secret: "refused-credential-reference"}, false)
	assert.False(t, ok, "an unreadable secret")
	time.Sleep(50 * time.Millisecond)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	assert.Empty(t, sink.events)
}
