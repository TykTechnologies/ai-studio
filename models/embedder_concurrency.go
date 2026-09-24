package models

import (
	"errors"
	"hash/fnv"
	"strings"

	"gorm.io/gorm"
)

// Embedders are found or created on demand (a datasource written with the
// legacy embed_* fields, a router naming an LLM). Two such writes at once
// must not both create: on Postgres the find-or-create runs in a transaction
// holding an advisory lock on the configuration, so the second waits and then
// finds the first's embedder. SQLite serialises writers on its own.

// embedderLockSpace keeps these advisory locks apart from others (the
// migration's embedderMigrationLockKey among them).
const embedderLockSpace = "embedder-config|"

// LockEmbedderConfig takes a transaction-scoped Postgres advisory lock on an
// embedder configuration. It must run inside a transaction; it is a no-op on
// other databases.
func LockEmbedderConfig(tx *gorm.DB, parts ...string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(embedderLockSpace + strings.Join(parts, "\x00")))
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(h.Sum64())).Error
}

// IsUniqueViolation reports whether err is a unique constraint violation, on
// Postgres or SQLite.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23505") || strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}

// maxNameAttempts bounds the retries when another write takes the name an
// embedder was about to get.
const maxNameAttempts = 5

// CreateWithDefaultName inserts e named "<prefix> · <model>" (with " (n)"
// when taken). Different configurations can share that default name, so a
// concurrent write may take it between the check and the insert; the insert
// then runs again with the next free name. Each attempt runs in a savepoint,
// so a failed insert does not abort the caller's transaction on Postgres.
//
// On Postgres it also holds an advisory lock on the default name until the
// caller's transaction ends, so creates that would pick the same name take
// turns (and each sees the names committed before it) instead of colliding.
// It must run inside a transaction.
func CreateWithDefaultName(tx *gorm.DB, e *Embedder, prefix, model string) error {
	if err := LockEmbedderConfig(tx, "name", prefix, model); err != nil {
		return err
	}
	var err error
	for attempt := 0; attempt < maxNameAttempts; attempt++ {
		var name string
		if name, err = UniqueEmbedderName(tx, prefix, model); err != nil {
			return err
		}
		e.Name = name
		e.ID = 0
		err = tx.Transaction(func(sp *gorm.DB) error { return e.Create(sp) })
		if err == nil || !IsUniqueViolation(err) {
			return err
		}
	}
	return err
}
