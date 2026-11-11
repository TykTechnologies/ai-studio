package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
	"github.com/rs/zerolog/log"
)

// Starting from last-known-good (features/ActiveActive.md, Phase 3): an edge
// that finds no control plane at start-up may serve the snapshot it last
// applied (applied_snapshot.go) instead of exiting, when
// EDGE_START_FROM_LAST_KNOWN_GOOD allows it.
//
// What bounds this is how long the edge has been without a control plane,
// not how old its configuration is: a configuration that has not changed for
// a month is current as long as a control plane keeps saying so. So the edge
// records its last contact with a control plane (a registration accepted, a
// heartbeat acknowledged) and the limit, EDGE_LAST_KNOWN_GOOD_MAX_AGE, is
// measured from that. During that time nobody could change or revoke
// anything on this edge.

// controlContactTopic is the sync_states row that holds the time of the
// edge's last contact with a control plane.
const controlContactTopic = "control.contact"

// The contact time decides whether a configuration that may since have been
// revoked is still served, so, like applied_at in the stored snapshot's row,
// it is authenticated: whoever can write the edge's database must not be
// able to push it forward. The row's sequence_number holds 63 bits of
//
//	HMAC-SHA256(k, topic || 0x00 || unix seconds)
//	k = HKDF-SHA256(ENCRYPTION_KEY, info "control-contact-v1")
//
// (no new column). A row without a valid tag counts as no contact recorded,
// and the age is then measured from applied_at.
const controlContactInfo = "control-contact-v1"

// controlContactSkew is how far ahead of the edge's clock a recorded time
// may be and still be believed. A time further in the future means the
// clock was set back since; it cannot show how long the edge has been on
// its own, and must not make it look as if no time had passed.
const controlContactSkew = 5 * time.Minute

func controlContactTag(key []byte, at time.Time) uint64 {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(controlContactTopic))
	mac.Write([]byte{0})
	mac.Write([]byte(strconv.FormatInt(at.UTC().Unix(), 10)))
	// 63 bits: database drivers refuse an unsigned value with the top bit
	// set, and the column is a signed 64-bit integer on Postgres.
	return binary.BigEndian.Uint64(mac.Sum(nil)[:8]) &^ (1 << 63)
}

// controlContactWriteInterval is the least time between two writes of the
// contact time: heartbeats arrive every 30 s by default, and a minute's
// resolution is plenty against a limit counted in hours.
const controlContactWriteInterval = time.Minute

// controlContactWrites holds, per database, when the contact time was last
// written (Unix nanoseconds).
var controlContactWrites sync.Map // dbKey -> *contactWrite

type contactWrite struct {
	mu   sync.Mutex
	last time.Time
}

// RecordControlContact notes that the edge had contact with a control plane
// at now, authenticated with the key of the database's snapshot store
// (PersistAppliedSnapshots; without one nothing is recorded). It writes to
// the database at most once per controlContactWriteInterval; a failed write
// is retried at the next contact.
func RecordControlContact(db *gorm.DB, now time.Time) error {
	sealer, ok := sealerOf(db, "the time of the last control contact")
	if !ok {
		return nil
	}
	now = now.UTC().Truncate(time.Second)
	v, _ := controlContactWrites.LoadOrStore(dbKey(db), &contactWrite{})
	w := v.(*contactWrite)
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.last.IsZero() && now.Sub(w.last) < controlContactWriteInterval {
		return nil
	}
	// For this topic sequence_number is not a sequence: it carries the tag
	// that authenticates last_sync_at (controlContactTag).
	row := &database.SyncState{
		Topic:          controlContactTopic,
		LastSyncAt:     now,
		SequenceNumber: controlContactTag(sealer.contactKey, now),
	}
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "topic"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_sync_at", "sequence_number", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return fmt.Errorf("record control contact: %w", err)
	}
	w.last = now
	return nil
}

// LastControlContact returns when the edge last had contact with a control
// plane, as recorded in its database; ok is false when it never recorded
// one, or when what is recorded does not carry a valid tag under the key of
// the database's snapshot store.
func LastControlContact(db *gorm.DB) (at time.Time, ok bool, err error) {
	sealer, registered := sealerOf(db, "the time of the last control contact (read)")
	if !registered {
		return time.Time{}, false, nil
	}
	return lastControlContact(db, sealer.contactKey)
}

func lastControlContact(db *gorm.DB, key []byte) (at time.Time, ok bool, err error) {
	var row database.SyncState
	err = db.Where("topic = ?", controlContactTopic).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read control contact: %w", err)
	}
	if row.LastSyncAt.IsZero() {
		return time.Time{}, false, nil
	}
	if row.SequenceNumber != controlContactTag(key, row.LastSyncAt) {
		log.Warn().Time("recorded", row.LastSyncAt).
			Msg("The recorded time of this edge's last contact with a control plane is not authentic (it was changed in the database, or written under another ENCRYPTION_KEY): ignoring it")
		return time.Time{}, false, nil
	}
	return row.LastSyncAt, true, nil
}

// LastKnownGood is the stored snapshot an edge may start from.
type LastKnownGood struct {
	*StoredSnapshot
	// LastContact is when the edge last had a control plane: the later of
	// its recorded last contact and the time the snapshot was applied.
	LastContact time.Time
}

// LastKnownGoodRefusal is why an edge may not start from its stored
// snapshot. The edge then exits, as it does without the option.
type LastKnownGoodRefusal struct {
	Reason string
	Err    error
}

func (r *LastKnownGoodRefusal) Error() string {
	if r.Err != nil {
		return r.Reason + ": " + r.Err.Error()
	}
	return r.Reason
}

func (r *LastKnownGoodRefusal) Unwrap() error { return r.Err }

// LoadLastKnownGood returns the stored snapshot if this edge may start from
// it, and a *LastKnownGoodRefusal saying why not otherwise:
//
//   - nothing is stored;
//   - it does not decrypt with this ENCRYPTION_KEY (or the row was changed);
//   - it was stored for another namespace than this edge's;
//   - the edge has been without a control plane for longer than maxAge
//     (0 means no limit);
//   - the snapshot was applied at a time that is in the future by the
//     edge's clock (the clock was set back), so no age can be worked out.
func LoadLastKnownGood(db *gorm.DB, encryptionKey, namespace string, maxAge time.Duration, now time.Time) (*LastKnownGood, error) {
	stored, err := LoadAppliedSnapshot(db, encryptionKey)
	if errors.Is(err, ErrNoStoredSnapshot) {
		return nil, &LastKnownGoodRefusal{Reason: "no configuration snapshot is stored in this edge's database (it has never applied one, or its database is new: an edge needs a persistent database to start without a control plane)"}
	}
	if err != nil {
		return nil, &LastKnownGoodRefusal{Reason: "the stored configuration snapshot cannot be read", Err: err}
	}
	if stored.Namespace != namespace {
		return nil, &LastKnownGoodRefusal{Reason: fmt.Sprintf("the stored configuration snapshot is for namespace %q, and this edge's EDGE_NAMESPACE is %q", stored.Namespace, namespace)}
	}

	// applied_at is authenticated with the snapshot. If it lies ahead of
	// this edge's clock, the clock has been set back and no age can be
	// worked out from it.
	if stored.AppliedAt.After(now.Add(controlContactSkew)) {
		return nil, &LastKnownGoodRefusal{Reason: fmt.Sprintf(
			"the stored configuration snapshot was applied at %s, which is in the future by this edge's clock (%s): the clock was set back, and how long the edge has been without a control plane cannot be known. Correct the edge's clock, or let the edge connect to a control plane once",
			stored.AppliedAt.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))}
	}
	lkg := &LastKnownGood{StoredSnapshot: stored, LastContact: stored.AppliedAt}
	sealer, err := newSnapshotSealer(encryptionKey)
	if err != nil {
		return nil, &LastKnownGoodRefusal{Reason: "the time of this edge's last contact with a control plane cannot be checked", Err: err}
	}
	contact, ok, err := lastControlContact(db, sealer.contactKey)
	if err != nil {
		return nil, &LastKnownGoodRefusal{Reason: "the time of this edge's last contact with a control plane cannot be read", Err: err}
	}
	switch {
	case ok && contact.After(now.Add(controlContactSkew)):
		// The clock stepped back since that contact: it says nothing about
		// how long ago it was. Fall back to applied_at.
		log.Warn().Time("recorded", contact).Time("now", now).
			Msg("The recorded last contact with a control plane is in the future by this edge's clock; ignoring it and measuring from when the snapshot was applied")
	case ok && contact.After(lkg.LastContact):
		lkg.LastContact = contact
	}
	// Within the tolerated skew a time may still be slightly ahead: the age
	// is then zero, never negative.
	if lkg.LastContact.After(now) {
		lkg.LastContact = now
	}
	if maxAge > 0 {
		if away := now.Sub(lkg.LastContact); away > maxAge {
			return nil, &LastKnownGoodRefusal{Reason: fmt.Sprintf(
				"this edge last had a control plane %s ago (%s), longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE (%s)",
				away.Round(time.Second), lkg.LastContact.UTC().Format(time.RFC3339), maxAge)}
		}
	}
	return lkg, nil
}

// restoringSnapshots holds, per database, the fingerprint of the stored
// snapshot the edge is starting from. Applying that snapshot again puts the
// tables and the in-memory configuration back; the stored row already holds
// it and is left as it is, with the time it was first applied.
var restoringSnapshots sync.Map // dbKey -> [sha256.Size]byte

// KeepStoredSnapshot tells the sync services on db that lkg is about to be
// applied again, from the database rather than from a control plane, so the
// stored row is not rewritten by it.
func KeepStoredSnapshot(db *gorm.DB, lkg *LastKnownGood) {
	if fp, ok := snapshotFingerprint(lkg.Snapshot); ok {
		restoringSnapshots.Store(dbKey(db), fp)
	}
}
