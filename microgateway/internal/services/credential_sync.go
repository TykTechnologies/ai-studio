package services

import (
	"context"
	"math/rand/v2"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

// Full credential sync (features/ActiveActive.md, Phase 4; Enterprise).
//
// With EDGE_CREDENTIAL_SYNC=full an edge keeps, in its own database, the
// hashed list of every App key of its namespace (database.SyncedCredential)
// and validates requests from it, so a key it has never served is accepted
// with no control plane to ask, and a revoked one is refused. The control
// plane sends the whole list when the edge has none and the changes since a
// watermark afterwards; the keys themselves never leave it.
//
// The list is trusted for EDGE_CREDENTIAL_SYNC_MAX_AGE after the last
// successful sync. Past that, and in every case the list cannot answer (a
// key that is not on it), the edge does what it does on demand: it asks the
// control plane.

// credentialSyncTopic is the sync_states row of the credential sync: the
// watermark of the last complete sync (SequenceNumber) and when it
// completed (LastSyncAt).
const credentialSyncTopic = "credentials.sync"

// ErrCredentialSyncUnavailable is returned by a CredentialSyncSource whose
// control plane does not offer the sync: one older than this feature, or a
// Community one. The edge then stays on demand.
var ErrCredentialSyncUnavailable = errors.New("the control plane does not offer credential sync")

// SyncedCredentialUpdate is one credential as the control plane sends it.
// The credential's ID is what identifies it: a regenerated key arrives as
// the same ID with a new hash, a change of state as the same ID and hash
// with Active flipped, and a deletion as a tombstone, the ID with Deleted
// set and nothing else (no hash, no App). Tombstones only come in a delta;
// in a full sync what is missing is what was deleted.
type SyncedCredentialUpdate struct {
	CredentialID uint
	SecretHash   string
	AppID        uint
	Active       bool
	Deleted      bool
	UpdatedAt    time.Time
}

// CredentialSyncPage is one page of a credential sync.
type CredentialSyncPage struct {
	Credentials []SyncedCredentialUpdate
	// Apps are the Apps of this page's credentials, as token validation
	// returns them for pull-on-miss.
	Apps []*pb.AppConfig
	// Full is true when the control plane is sending the whole list (it
	// was asked for it, or could not serve the changes since the edge's
	// watermark): rows the sync does not bring are removed when it ends.
	Full bool
	// Last marks the final page, which carries Watermark: an opaque value
	// to send back as since next time (0: none, ask for everything). It is
	// never compared with another.
	Last      bool
	Watermark uint64
	// HashVersion is the version of the hashes on this page (credhash's
	// prefix without the colon). A page in a version this edge does not
	// compute is not applied.
	HashVersion string
}

// credentialHashVersion is the version of credential hash this edge
// computes and stores (credhash.Prefix without its colon).
var credentialHashVersion = strings.TrimSuffix(credhash.Prefix, ":")

// CredentialHashVersion is the version of the credential hashes this edge
// computes ("v1"): what it asks the control plane for.
func CredentialHashVersion() string { return credentialHashVersion }

// ErrCredentialSyncNotRegistered is returned by a CredentialSyncSource whose
// control plane does not know this edge (its registration was lost, or the
// sync raced it): the edge registers again, then asks again. It is not a
// control plane without the sync: the edge does not fall back to on demand.
var ErrCredentialSyncNotRegistered = errors.New("credential sync: the control plane does not know this edge; it registers again")

// CredentialSyncRegistrar is implemented by a CredentialSyncSource that can
// register the edge again when the control plane does not know it.
type CredentialSyncRegistrar interface {
	Reregister() error
}

// reregisterInterval is the least time between two registrations the sync
// asks for: a control plane that keeps answering "register first" after a
// registration it accepted must not be asked in a loop.
const reregisterInterval = time.Minute

// ErrCredentialHashVersion is returned when the control plane sends hashes
// in a version this edge does not compute: nothing of that sync is kept as
// its result (the watermark and the time of the last success stay as they
// were).
var ErrCredentialHashVersion = errors.New("credential sync: the control plane's credential hashes are in a version this edge does not compute")

// CredentialSyncSource is the control plane's side of the sync: it calls
// onPage for each page of the credentials changed since the watermark (0
// asks for everything), in order, and returns when the last page was handed
// over or with the first error.
type CredentialSyncSource interface {
	SyncCredentials(ctx context.Context, since uint64, onPage func(*CredentialSyncPage) error) error
}

// fullCredentialSyncAvailable says whether this build may run full
// credential sync: Enterprise builds only (credential_sync_enterprise.go).
// A variable so that tests of the mechanism run in every build.
var fullCredentialSyncAvailable = fullCredentialSyncBuild

// Validation sources, as microgateway_credential_validations_total counts
// them.
const (
	// ValidationLocal: answered from the synced list, no control plane call.
	ValidationLocal = "local"
	// ValidationControl: the key was not on a trusted list (or the edge is
	// on demand) and the control plane, or its cached answer, decided.
	ValidationControl = "control"
	// ValidationStale: the list is past EDGE_CREDENTIAL_SYNC_MAX_AGE, so the
	// edge fell back to asking, as on demand.
	ValidationStale = "stale"
)

// CredentialSync is an edge's synced credential list: the store, the sync
// with the control plane and the answers validation takes from it.
type CredentialSync struct {
	db     *gorm.DB
	maxAge time.Duration

	// syncMu makes syncs run one at a time.
	syncMu sync.Mutex
	// unavailable is set once the control plane said it does not offer the
	// sync; the edge stops asking until it reconnects (Reset).
	unavailable atomic.Bool
	warned      atomic.Bool
	// failing is set while syncs fail, so the failure is logged once.
	failing atomic.Bool
	// lastSuccess is when the last sync completed (Unix nanoseconds; 0 when
	// none has, in this database's lifetime).
	lastSuccess atomic.Int64
	// count is the number of usable credentials on the list.
	count atomic.Int64
	// kick wakes Run: at most one sync is pending behind the running one.
	kick chan struct{}
	// Syncs started by credential.changed events (KickForChange): at most
	// one is scheduled at a time, after a random delay of up to
	// eventSyncMaxDelay, and no sooner than eventSyncMinInterval after the
	// last one. Zero values mean the defaults (tests shorten them).
	eventPending         atomic.Bool
	eventMu              sync.Mutex
	lastEventKick        time.Time
	eventSyncMaxDelay    time.Duration
	eventSyncMinInterval time.Duration
	// lastReregister is when Run last registered the edge again after a
	// "register first" answer (Run's goroutine only).
	lastReregister time.Time

	local, control, stale atomic.Uint64
	// Syncs asked for, by trigger (KickFor).
	byRegistration, byHeartbeat, byEvent atomic.Uint64
}

// Triggers of a sync, as microgateway_credential_sync_calls_total counts
// them.
const (
	// SyncTriggerRegistration: a control plane accepted the edge's
	// registration (at start, after a reconnect, after a move to another
	// control endpoint).
	SyncTriggerRegistration = "registration"
	// SyncTriggerHeartbeat: a control plane acknowledged a heartbeat.
	SyncTriggerHeartbeat = "heartbeat"
	// SyncTriggerEvent: a credential.changed event for a key on the list
	// (KickForChange).
	SyncTriggerEvent = "event"
)

// CredentialSyncEnabled reports whether hs asks for full credential sync and
// this build may run it. A Community edge given EDGE_CREDENTIAL_SYNC=full
// logs that it is an Enterprise option and stays on demand.
func CredentialSyncEnabled(hs config.HubSpokeConfig) bool {
	if hs.CredentialSync != config.CredentialSyncFull {
		return false
	}
	if !fullCredentialSyncAvailable {
		log.Warn().Msg("EDGE_CREDENTIAL_SYNC=full is an Enterprise option: this Community edge validates keys on demand")
		return false
	}
	return true
}

// NewCredentialSync returns the synced credential list on db. The time of
// the last successful sync is read from the database, so a restarted edge
// knows how old its list is.
func NewCredentialSync(db *gorm.DB, maxAge time.Duration) (*CredentialSync, error) {
	s := &CredentialSync{db: db, maxAge: maxAge, kick: make(chan struct{}, 1)}
	// Limit(1).Find, not First: no row yet is the normal state of an edge
	// that has not synced, not an error to log.
	var state database.SyncState
	if err := db.Where("topic = ?", credentialSyncTopic).Limit(1).Find(&state).Error; err != nil {
		return nil, fmt.Errorf("read credential sync state: %w", err)
	}
	if !state.LastSyncAt.IsZero() {
		s.lastSuccess.Store(state.LastSyncAt.UnixNano())
	}
	if err := s.recount(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *CredentialSync) recount() error {
	var n int64
	if err := s.db.Model(&database.SyncedCredential{}).Where("active = ? AND deleted = ?", true, false).Count(&n).Error; err != nil {
		return fmt.Errorf("count synced credentials: %w", err)
	}
	s.count.Store(n)
	return nil
}

// LastSuccess is when the last sync completed; zero if none has.
func (s *CredentialSync) LastSuccess() time.Time {
	if ns := s.lastSuccess.Load(); ns != 0 {
		return time.Unix(0, ns)
	}
	return time.Time{}
}

// Count is the number of usable credentials on the list.
func (s *CredentialSync) Count() int64 { return s.count.Load() }

// Validations returns how many validations each source answered.
func (s *CredentialSync) Validations() map[string]uint64 {
	return map[string]uint64{
		ValidationLocal:   s.local.Load(),
		ValidationControl: s.control.Load(),
		ValidationStale:   s.stale.Load(),
	}
}

// SyncCalls returns how many syncs were asked for, by trigger, since the
// process started. A sync asked for while one runs is folded into the next
// one, so the sum can exceed the syncs run.
func (s *CredentialSync) SyncCalls() map[string]uint64 {
	return map[string]uint64{
		SyncTriggerRegistration: s.byRegistration.Load(),
		SyncTriggerHeartbeat:    s.byHeartbeat.Load(),
		SyncTriggerEvent:        s.byEvent.Load(),
	}
}

// Trusted reports whether the list may answer at now: a sync has completed,
// and not longer ago than the max age.
func (s *CredentialSync) Trusted(now time.Time) bool {
	last := s.LastSuccess()
	if last.IsZero() {
		return false
	}
	return s.maxAge <= 0 || now.Sub(last) <= s.maxAge
}

// Lookup returns the list's row for a credential hash; found is false when
// the key is not on the list.
func (s *CredentialSync) Lookup(hash string) (row database.SyncedCredential, found bool, err error) {
	err = s.db.Where("secret_hash = ?", hash).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("look up synced credential: %w", err)
	}
	return row, true, nil
}

// counted notes which source answered a validation.
func (s *CredentialSync) counted(source string) {
	switch source {
	case ValidationLocal:
		s.local.Add(1)
	case ValidationStale:
		s.stale.Add(1)
	default:
		s.control.Add(1)
	}
}

// Learned records what the control plane said about a key the list could
// not answer for: an accepted key is added (it was created a moment ago and
// the next delta will bring it with its ID), and a rejection marks the row
// the list has, if any, unusable. The control plane's word always wins.
func (s *CredentialSync) Learned(hash string, appID uint, valid bool) {
	now := time.Now().UTC()
	var err error
	if valid {
		row := database.SyncedCredential{SecretHash: hash, AppID: appID, Active: true, UpdatedAt: now, SyncedAt: now}
		err = s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "secret_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"app_id", "active", "deleted", "synced_at"}),
		}).Create(&row).Error
	} else {
		res := s.db.Model(&database.SyncedCredential{}).Where("secret_hash = ?", hash).
			Updates(map[string]interface{}{"active": false, "synced_at": now})
		if err = res.Error; err == nil && res.RowsAffected == 0 {
			// A key the list does not hold: nothing changed to count.
			return
		}
	}
	if err != nil {
		log.Warn().Err(err).Msg("Credential sync: could not record the control plane's answer")
		return
	}
	if err := s.recount(); err != nil {
		log.Warn().Err(err).Msg("Credential sync: could not count the list")
	}
}

// Unavailable reports whether the control plane last said it does not offer
// the sync (until Reset): the edge then validates on demand.
func (s *CredentialSync) Unavailable() bool { return s.unavailable.Load() }

// Reset forgets that the control plane did not offer the sync: the edge has
// reconnected, possibly to another control plane or an upgraded one.
func (s *CredentialSync) Reset() { s.unavailable.Store(false) }

// Defaults for the spacing of event-driven syncs (KickForChange).
const (
	// defaultEventSyncMaxDelay spreads the edges that hold a changed key
	// over half a second instead of all calling the hub at once.
	defaultEventSyncMaxDelay = 500 * time.Millisecond
	// defaultEventSyncMinInterval bounds one edge to about one
	// event-driven sync a second under a stream of key changes.
	defaultEventSyncMinInterval = time.Second
)

// KickForChange is the edge's answer to a credential.changed event for
// hash: a sync, so the change arrives with the credential's ID, but only
// when the list holds hash. A key the list does not hold is another
// namespace's, or one created since the last sync; neither needs a sync
// now (a new key's first use asks the control plane, and the heartbeat's
// sync brings it), so one key change does not make every edge call the
// hub. The sync is spaced (a random delay, and a least interval between
// event-driven syncs on this edge); heartbeat syncs are not. Reports
// whether a sync was asked for.
func (s *CredentialSync) KickForChange(hash string) bool {
	_, found, err := s.Lookup(hash)
	if err != nil || !found {
		return false
	}
	if !s.eventPending.CompareAndSwap(false, true) {
		return true // one is scheduled already: it will bring this change too
	}
	maxDelay, minInterval := s.eventSyncMaxDelay, s.eventSyncMinInterval
	if maxDelay <= 0 {
		maxDelay = defaultEventSyncMaxDelay
	}
	if minInterval <= 0 {
		minInterval = defaultEventSyncMinInterval
	}
	delay := rand.N(maxDelay)
	s.eventMu.Lock()
	if wait := time.Until(s.lastEventKick.Add(minInterval)); wait > delay {
		delay = wait
	}
	s.eventMu.Unlock()
	time.AfterFunc(delay, func() {
		s.eventMu.Lock()
		s.lastEventKick = time.Now()
		s.eventMu.Unlock()
		s.eventPending.Store(false)
		s.KickFor(SyncTriggerEvent)
	})
	return true
}

// KickFor is Kick, counted under trigger (SyncCalls).
func (s *CredentialSync) KickFor(trigger string) {
	switch trigger {
	case SyncTriggerRegistration:
		s.byRegistration.Add(1)
	case SyncTriggerHeartbeat:
		s.byHeartbeat.Add(1)
	case SyncTriggerEvent:
		s.byEvent.Add(1)
	}
	s.Kick()
}

// Kick asks Run for a sync and returns at once. The edge calls it on every
// contact with a control plane: a registration accepted (after Reset) and
// each heartbeat acknowledged, which is the "delta on each heartbeat tick".
// Kicks that arrive while a sync runs are folded into one sync after it.
func (s *CredentialSync) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Run syncs from source each time Kick is called, one sync at a time, until
// ctx is done. A sync that takes longer than timeout is given up; the next
// kick starts another from the watermark the edge still has. A control plane
// that does not offer the sync is not asked again until Reset.
func (s *CredentialSync) Run(ctx context.Context, source CredentialSyncSource, timeout time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
		}
		syncCtx, cancel := context.WithTimeout(ctx, timeout)
		err := s.Sync(syncCtx, source)
		cancel()
		switch {
		case err == nil, errors.Is(err, ErrCredentialSyncUnavailable):
			s.failing.Store(false)
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrCredentialSyncNotRegistered):
			// Register, then ask again: the registration's contact kicks
			// the next sync. At most once a minute.
			registrar, ok := source.(CredentialSyncRegistrar)
			if !ok || time.Since(s.lastReregister) < reregisterInterval {
				break
			}
			s.lastReregister = time.Now()
			log.Warn().Msg("Credential sync: the control plane does not know this edge; registering again")
			if rerr := registrar.Reregister(); rerr != nil {
				log.Warn().Err(rerr).Msg("Credential sync: registering again failed; the sync is retried on the next heartbeat")
			}
		default:
			// Said once per run of failures: during an outage every
			// heartbeat that still gets through would repeat it.
			if s.failing.CompareAndSwap(false, true) {
				log.Warn().Err(err).Time("last_success", s.LastSuccess()).
					Msg("Credential sync failed; the edge keeps the list it has and tries again on the next heartbeat")
			}
		}
	}
}

// Sync brings the list up to date from source: everything when the edge has
// no watermark, the changes since it otherwise. It applies each page as it
// arrives (upserts, so a repeated or overlapping page is harmless) and
// records the watermark and the time only when the whole stream was
// applied. A control plane that does not offer the sync is logged once and
// not asked again until Reset.
func (s *CredentialSync) Sync(ctx context.Context, source CredentialSyncSource) error {
	if s.unavailable.Load() {
		return ErrCredentialSyncUnavailable
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	var state database.SyncState
	err := s.db.Where("topic = ?", credentialSyncTopic).Limit(1).Find(&state).Error
	if err != nil {
		return fmt.Errorf("read credential sync state: %w", err)
	}
	since := state.SequenceNumber

	started := time.Now().UTC()
	full := since == 0
	var watermark uint64
	finished := false
	storedApps := map[uint32]struct{}{}
	err = source.SyncCredentials(ctx, since, func(page *CredentialSyncPage) error {
		if page.HashVersion != "" && page.HashVersion != credentialHashVersion {
			return fmt.Errorf("%w (got %q, this edge computes %q)", ErrCredentialHashVersion, page.HashVersion, credentialHashVersion)
		}
		if page.Full {
			full = true
		}
		if err := s.applyPage(page, started, storedApps); err != nil {
			return err
		}
		if page.Last {
			finished = true
			watermark = page.Watermark
		}
		return nil
	})
	if errors.Is(err, ErrCredentialSyncUnavailable) {
		s.unavailable.Store(true)
		if s.warned.CompareAndSwap(false, true) {
			log.Warn().Msg("The control plane does not offer credential sync (it is older than this edge, or a Community control plane): this edge validates keys on demand")
		}
		return err
	}
	if err != nil {
		return fmt.Errorf("credential sync: %w", err)
	}
	if !finished {
		return errors.New("credential sync: the stream ended without its last page")
	}

	// Rows the edge added on the control plane's word (Learned) have no
	// credential ID, so a delta cannot remove or replace them: a key used
	// once and then deleted or regenerated would stay accepted until the
	// next full sync. Any such row learned before this sync started is
	// dropped now; the sync has brought the key with its ID if it still
	// exists, and otherwise the next request asks again.
	if err := s.db.Where("credential_id = 0 AND synced_at < ?", started).Delete(&database.SyncedCredential{}).Error; err != nil {
		return fmt.Errorf("credential sync: remove keys learned before the sync: %w", err)
	}

	// A full sync brought every credential there is: what it did not bring
	// no longer exists on the control plane.
	if full {
		if err := s.db.Where("synced_at < ?", started).Delete(&database.SyncedCredential{}).Error; err != nil {
			return fmt.Errorf("credential sync: remove credentials the full sync did not bring: %w", err)
		}
		// Fingerprints of Apps the full sync did not bring only take room.
		if err := pruneFingerprints(s.db, storedApps); err != nil {
			return fmt.Errorf("credential sync: remove fingerprints of Apps the full sync did not bring: %w", err)
		}
		refreshSyncStatistics(s.db)
	}
	now := time.Now().UTC()
	row := database.SyncState{Topic: credentialSyncTopic, SequenceNumber: watermark, LastSyncAt: now}
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "topic"}},
		DoUpdates: clause.AssignmentColumns([]string{"sequence_number", "last_sync_at", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("credential sync: record the watermark: %w", err)
	}
	s.lastSuccess.Store(now.UnixNano())
	return s.recount()
}

// applyPage applies a page in one transaction: its Apps, then each
// credential by its ID.
//
//   - An App is stored once per sync, and not at all when it is unchanged
//     since the sync last stored it (storeApps).
//   - A tombstone (Deleted, no hash) removes every row of that credential.
//     One for a credential this edge never held removes nothing.
//   - Any other credential replaces what the edge holds for that ID: rows
//     of the same ID under another hash are removed first (the key was
//     regenerated: the old one must stop working now, not at the next full
//     sync), then the row is upserted by its hash. A row this edge added on
//     the control plane's word (Learned, no ID yet) is taken over by the
//     upsert.
//
// Applying a page again changes nothing.
func (s *CredentialSync) applyPage(page *CredentialSyncPage, syncedAt time.Time, storedApps map[uint32]struct{}) error {
	var apps []*pb.AppConfig
	for _, app := range page.Apps {
		if app == nil {
			continue
		}
		if _, done := storedApps[app.Id]; done {
			continue
		}
		storedApps[app.Id] = struct{}{}
		apps = append(apps, app)
	}
	rows := make([]database.SyncedCredential, 0, len(page.Credentials))
	var tombstones []uint
	replaced := map[uint]string{} // credential ID -> the hash it has now
	for _, c := range page.Credentials {
		if c.SecretHash == "" {
			if c.Deleted && c.CredentialID != 0 {
				tombstones = append(tombstones, c.CredentialID)
			}
			continue
		}
		if c.CredentialID != 0 {
			replaced[c.CredentialID] = c.SecretHash
		}
		rows = append(rows, database.SyncedCredential{
			SecretHash:   c.SecretHash,
			CredentialID: c.CredentialID,
			AppID:        c.AppID,
			Active:       c.Active,
			Deleted:      c.Deleted,
			UpdatedAt:    c.UpdatedAt.UTC(),
			SyncedAt:     syncedAt,
		})
	}
	if len(apps) == 0 && len(tombstones) == 0 && len(rows) == 0 {
		return nil
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := storeApps(tx, apps); err != nil {
			return err
		}
		for i := 0; i < len(tombstones); i += credentialBatch {
			ids := tombstones[i:min(i+credentialBatch, len(tombstones))]
			if err := tx.Where("credential_id IN ?", ids).Delete(&database.SyncedCredential{}).Error; err != nil {
				return fmt.Errorf("remove deleted credentials: %w", err)
			}
		}
		if len(rows) == 0 {
			return nil
		}
		// The rows of these credentials under a hash they no longer have.
		ids := make([]uint, 0, len(replaced))
		hashes := make([]string, 0, len(replaced))
		for id, hash := range replaced {
			ids = append(ids, id)
			hashes = append(hashes, hash)
		}
		for i := 0; i < len(ids); i += credentialBatch {
			j := min(i+credentialBatch, len(ids))
			if err := tx.Where("credential_id IN ? AND secret_hash NOT IN ?", ids[i:j], hashes[i:j]).
				Delete(&database.SyncedCredential{}).Error; err != nil {
				return fmt.Errorf("remove replaced credential hashes: %w", err)
			}
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "secret_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"credential_id", "app_id", "active", "deleted", "updated_at", "synced_at"}),
		}).CreateInBatches(rows, credentialBatch).Error
	})
	if err != nil {
		// Not stored: a retry of this sync stores them.
		for _, app := range apps {
			delete(storedApps, app.Id)
		}
	}
	return err
}

// refreshSyncStatistics runs ANALYZE on the tables a full sync rewrites,
// on PostgreSQL only and only after a full sync (a delta sync changes few
// rows). A full sync can rewrite every row of them, and the planner then
// works from statistics taken before: the next sync of 50,000 credentials
// took 10 s instead of 3.4 s until autovacuum caught up (measured). ANALYZE
// samples at most 30,000 rows per table, so its cost is bounded whatever
// the size (TestCredentialSync_FullSyncOf50000WhileValidating measures
// it). SQLite needs nothing: its planner does not use these statistics.
// A failure is logged and the sync stands.
func refreshSyncStatistics(db *gorm.DB) {
	if db.Dialector.Name() != "postgres" {
		return
	}
	if err := db.Exec("ANALYZE synced_credentials, synced_app_fingerprints, apps, app_llms").Error; err != nil {
		log.Warn().Err(err).Msg("Credential sync: could not refresh the table statistics")
	}
}

// pruneFingerprints removes the fingerprints of Apps not in keep. It reads
// the IDs and deletes in batches: a DELETE with a subquery against
// synced_credentials took seconds on Postgres at 50,000 rows (measured).
func pruneFingerprints(db *gorm.DB, keep map[uint32]struct{}) error {
	var held []uint32
	if err := db.Model(&database.SyncedAppFingerprint{}).Pluck("app_id", &held).Error; err != nil {
		return err
	}
	var gone []uint32
	for _, id := range held {
		if _, ok := keep[id]; !ok {
			gone = append(gone, id)
		}
	}
	for i := 0; i < len(gone); i += credentialBatch {
		if err := db.Where("app_id IN ?", gone[i:min(i+credentialBatch, len(gone))]).Delete(&database.SyncedAppFingerprint{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// appFingerprint is a hash of the App as the control plane sent it: its
// deterministic protobuf encoding, so every field counts, a field added to
// the message later included.
func appFingerprint(app *pb.AppConfig) (string, error) {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(app)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// storeApps upserts the Apps of a page, and their LLM grants, in tx, as
// pull-on-miss stores an App (the same columns), with is_active written
// explicitly, false included: on insert a struct's false would be replaced
// by the column's default (true), and an inactive App's keys must be
// refused.
//
// An App is skipped when its fingerprint is the one the sync last stored
// for it and its row still has the updated_at the sync wrote then, so a
// repeat full sync (every 15 minutes, and after a reconnect to another
// replica) writes only what changed. Any other write of the row changes
// updated_at, and the App is stored again.
func storeApps(tx *gorm.DB, apps []*pb.AppConfig) error {
	if len(apps) == 0 {
		return nil
	}
	prints := make(map[uint32]string, len(apps))
	ids := make([]uint32, 0, len(apps))
	for _, app := range apps {
		fp, err := appFingerprint(app)
		if err != nil {
			return fmt.Errorf("fingerprint app %d: %w", app.Id, err)
		}
		prints[app.Id] = fp
		ids = append(ids, app.Id)
	}
	unchanged := map[uint32]bool{}
	for i := 0; i < len(ids); i += credentialBatch {
		var held []struct {
			AppID       uint32
			Fingerprint string
		}
		if err := tx.Table("synced_app_fingerprints AS f").Select("f.app_id, f.fingerprint").
			Joins("JOIN apps a ON a.id = f.app_id AND a.updated_at = f.app_updated_at AND a.deleted_at IS NULL").
			Where("f.app_id IN ?", ids[i:min(i+credentialBatch, len(ids))]).Scan(&held).Error; err != nil {
			return fmt.Errorf("read app fingerprints: %w", err)
		}
		for _, h := range held {
			if prints[h.AppID] == h.Fingerprint {
				unchanged[h.AppID] = true
			}
		}
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	var (
		rows    []map[string]interface{}
		changed []uint32
		grants  []database.AppLLM
		fps     []database.SyncedAppFingerprint
	)
	for _, app := range apps {
		if unchanged[app.Id] {
			continue
		}
		changed = append(changed, app.Id)
		row := map[string]interface{}{
			"id":                app.Id,
			"created_at":        app.CreatedAt.AsTime(),
			"updated_at":        now,
			"deleted_at":        nil,
			"name":              app.Name,
			"description":       app.Description,
			"owner_email":       app.OwnerEmail,
			"user_id":           app.UserId,
			"is_active":         app.IsActive,
			"monthly_budget":    app.MonthlyBudget,
			"budget_start_date": nil,
			"rate_limit_rpm":    app.RateLimitRpm,
			"namespace":         app.Namespace,
			"metadata":          nil,
		}
		if app.BudgetStartDate != "" {
			if start, err := time.Parse(time.RFC3339, app.BudgetStartDate); err == nil {
				row["budget_start_date"] = start
			}
		}
		if app.Metadata != "" {
			row["metadata"] = database.JSON(app.Metadata)
		}
		rows = append(rows, row)
		for _, llm := range app.LlmIds {
			grants = append(grants, database.AppLLM{AppID: uint(app.Id), LLMID: uint(llm), IsActive: true, CreatedAt: now})
		}
		fps = append(fps, database.SyncedAppFingerprint{AppID: uint(app.Id), Fingerprint: prints[app.Id], AppUpdatedAt: now})
	}
	if len(changed) == 0 {
		return nil
	}
	for i := 0; i < len(rows); i += appBatch {
		if err := tx.Table("apps").Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"updated_at", "deleted_at", "name", "description", "owner_email",
				"user_id", "is_active", "monthly_budget", "budget_start_date", "rate_limit_rpm", "namespace", "metadata"}),
		}).Create(rows[i:min(i+appBatch, len(rows))]).Error; err != nil {
			return fmt.Errorf("store apps: %w", err)
		}
	}
	for i := 0; i < len(changed); i += credentialBatch {
		if err := tx.Exec("DELETE FROM app_llms WHERE app_id IN ?", changed[i:min(i+credentialBatch, len(changed))]).Error; err != nil {
			return fmt.Errorf("clear app_llms: %w", err)
		}
	}
	if len(grants) > 0 {
		if err := tx.CreateInBatches(grants, credentialBatch).Error; err != nil {
			return fmt.Errorf("store app_llms: %w", err)
		}
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "app_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"fingerprint", "app_updated_at"}),
	}).CreateInBatches(fps, credentialBatch).Error
}

// appBatch is how many Apps one statement of storeApps carries (15 columns
// each): well under SQLite's limit on bound parameters.
const appBatch = 100

// credentialBatch is how many rows one statement of applyPage carries: well
// under SQLite's limit on bound parameters.
const credentialBatch = 200
