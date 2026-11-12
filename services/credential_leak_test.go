package services

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/studiodb"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// lockedBuffer is a log sink safe for concurrent writers.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// An App key must not leave the credentials table in clear. This creates an
// App (and with it a credential), activates it, replaces its secret, fails
// to create a duplicate, and migrates rows from before the change, with
// Studio's own gorm logger at debug (every statement logged, as LOG_LEVEL=
// debug does) and every bus event captured (what plugins receive and what
// the cluster event log relays to the other replicas). None of the secrets
// appears in the SQL log or in an event.
func TestAppKeyIsNotInSQLLogsOrEvents(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credential-leak-test-key")
	t.Setenv("DB_LOG_SQL_PARAMS", "")

	prev := logger.Log
	t.Cleanup(func() { logger.Use(prev) })
	logs := &lockedBuffer{}
	logger.Use(zerolog.New(logs).Level(zerolog.DebugLevel))

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "leak.db")), logger.GetGormConfig())
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(db))

	svc := NewService(db)
	bus := eventbridge.NewBus()
	svc.SetEventBus(bus)
	svc.SystemEvents = NewSystemEventEmitter(bus, "test")
	var mu sync.Mutex
	var events []string
	bus.SubscribeAll(func(ev eventbridge.Event) {
		raw, err := json.Marshal(ev)
		require.NoError(t, err)
		mu.Lock()
		events = append(events, string(raw)+" "+string(ev.Payload))
		mu.Unlock()
	})

	owner := &models.User{Email: "leak@example.com", Name: "Leak"}
	require.NoError(t, db.Create(owner).Error)

	// Create: the App and its credential.
	app, err := svc.CreateApp("Leak App", "", owner.ID, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	loaded, err := svc.GetAppByID(app.ID)
	require.NoError(t, err)
	first := loaded.Credential.Secret
	require.NotEmpty(t, first)
	require.False(t, strings.HasPrefix(first, "$ENC/"), "readers get the key itself")
	svc.SystemEvents.EmitAppCreated(loaded, loaded.ID, owner.ID)

	// Update: activation, then a new secret, and the App saved with its
	// credential loaded.
	require.NoError(t, svc.ActivateCredential(loaded.CredentialID))
	cred, err := svc.GetCredentialByID(loaded.CredentialID)
	require.NoError(t, err)
	fresh, err := models.NewCredential()
	require.NoError(t, err)
	second := fresh.Secret
	cred.Secret = second
	require.NoError(t, svc.UpdateCredential(cred))
	loaded, err = svc.GetAppByID(app.ID)
	require.NoError(t, err)
	require.Equal(t, second, loaded.Credential.Secret)
	loaded.Description = "changed"
	require.NoError(t, loaded.Update(db))
	svc.SystemEvents.EmitAppUpdated(loaded, loaded.ID, owner.ID)
	svc.SystemEvents.EmitAppApproved(loaded, loaded.ID, owner.ID)

	// A statement that fails is logged with its error: a second
	// credential with the same secret.
	require.Error(t, (&models.Credential{KeyID: "duplicate", Secret: second}).Create(db))

	// A lookup by the key, found and not found.
	found, err := svc.GetCredentialBySecret(second)
	require.NoError(t, err)
	require.Equal(t, cred.ID, found.ID)
	_, err = svc.GetCredentialBySecret(first)
	require.Error(t, err)

	// The migration of rows from before the change.
	legacy := []string{"legacy-plaintext-key-one", "legacy-plaintext-key-two"}
	quiet := db.Session(&gorm.Session{Logger: db.Logger.LogMode(1)}) // the old code's own inserts are not under test
	for i, s := range legacy {
		require.NoError(t, quiet.Exec("INSERT INTO credentials (key_id, secret, active, created_at, updated_at) VALUES (?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", "legacy-"+string(rune('a'+i)), s).Error)
	}
	res, err := models.MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	require.Equal(t, 2, res.Migrated)

	sqlLog := logs.String()
	require.Contains(t, sqlLog, "INSERT INTO `credentials`", "the SQL log is on: the credential insert is in it")
	require.Contains(t, sqlLog, "UPDATE `credentials`")
	require.Contains(t, sqlLog, "secret_hash")
	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(events), 3, "the App events were captured")
	all := strings.Join(events, "\n")
	require.Contains(t, all, "Leak App")
	for _, secret := range append([]string{first, second}, legacy...) {
		assert.NotContains(t, sqlLog, secret, "an App key is in the SQL log")
		assert.NotContains(t, all, secret, "an App key is in an event")
	}
}
