package database

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsStorageFull(t *testing.T) {
	for _, msg := range []string{
		"database or disk is full",
		"failed to commit sync transaction: database or disk is full",
		`ERROR: could not extend file "base/16384/2619": No space left on device (SQLSTATE 53100)`,
		"write /var/lib/x: no space left on device",
	} {
		assert.True(t, IsStorageFull(errors.New(msg)), msg)
	}
	for _, msg := range []string{"database is locked", "UNIQUE constraint failed: llms.slug", "context deadline exceeded"} {
		assert.False(t, IsStorageFull(errors.New(msg)), msg)
	}
	assert.False(t, IsStorageFull(nil))
}

// A write refused for lack of room marks its kind as failing, once in the
// log however often it repeats; a write of that kind that succeeds clears
// it, once in the log. Errors of other kinds say nothing about room.
func TestNoteWrite(t *testing.T) {
	resetStorageStatus()
	t.Cleanup(resetStorageStatus)
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })
	full := fmt.Errorf("commit: %w", errors.New("database or disk is full"))

	is, _, failing := StorageStatus()
	require.False(t, is)
	require.Empty(t, failing)

	// Another kind of error is returned as it is and changes nothing.
	other := errors.New("UNIQUE constraint failed")
	assert.Equal(t, other, NoteWrite(WriteConfiguration, other))
	is, _, _ = StorageStatus()
	assert.False(t, is)
	assert.NoError(t, NoteWrite(WriteConfiguration, nil))

	for i := 0; i < 5; i++ {
		assert.Equal(t, full, NoteWrite(WriteConfiguration, full), "the error comes back unchanged")
	}
	assert.Equal(t, full, NoteWrite(WriteControlContact, full))
	is, since, failing := StorageStatus()
	assert.True(t, is)
	assert.False(t, since.IsZero())
	assert.Equal(t, []string{WriteConfiguration, WriteControlContact}, failing)
	assert.Equal(t, map[string]uint64{WriteConfiguration: 5, WriteControlContact: 1}, StorageFullFailures())
	assert.Equal(t, 2, strings.Count(buf.String(), "has no room left"), "once per kind: %s", buf.String())
	assert.Equal(t, 2, strings.Count(buf.String(), `"level":"error"`))

	// A different error while full does not clear it.
	_ = NoteWrite(WriteConfiguration, other)
	is, _, _ = StorageStatus()
	assert.True(t, is)

	// Room again, kind by kind.
	assert.NoError(t, NoteWrite(WriteConfiguration, nil))
	is, _, failing = StorageStatus()
	assert.True(t, is, "the control contact is still not stored")
	assert.Equal(t, []string{WriteControlContact}, failing)
	assert.NoError(t, NoteWrite(WriteControlContact, nil))
	assert.NoError(t, NoteWrite(WriteControlContact, nil))
	is, since, failing = StorageStatus()
	assert.False(t, is)
	assert.True(t, since.IsZero())
	assert.Empty(t, failing)
	assert.Equal(t, 2, strings.Count(buf.String(), "has room again"), "once per kind")
	assert.Equal(t, uint64(5), StorageFullFailures()[WriteConfiguration], "the count of refused writes stays")

	// Full again: said again.
	_ = NoteWrite(WriteConfiguration, full)
	assert.Equal(t, 3, strings.Count(buf.String(), "has no room left"))
}
