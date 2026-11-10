package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
	"google.golang.org/protobuf/proto"
)

// An edge keeps the snapshot it last applied in its own database
// (database.AppliedSnapshot), encrypted, so that it can later start from it
// when no control plane answers (features/ActiveActive.md, Phase 3). The row
// is written in the transaction that applies the snapshot: a failed apply
// leaves the previous row, and the row never disagrees with the tables.
//
// Encryption: AES-256-GCM with a random nonce, under
//
//	k = HKDF-SHA256(ENCRYPTION_KEY, info "applied-snapshot-v1")
//
// The row's other columns are authenticated with the snapshot (as additional
// data), so a row whose checksum, version, namespace or time was changed in
// the database no longer opens.

// appliedSnapshotInfo is the HKDF info string of the snapshot key.
const appliedSnapshotInfo = "applied-snapshot-v1"

// snapshotSealer encrypts and decrypts stored snapshots.
type snapshotSealer struct {
	aead cipher.AEAD
}

func newSnapshotSealer(encryptionKey string) (*snapshotSealer, error) {
	if encryptionKey == "" {
		return nil, errors.New("applied snapshot: ENCRYPTION_KEY is empty")
	}
	k, err := hkdf.Key(sha256.New, []byte(encryptionKey), nil, appliedSnapshotInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("applied snapshot: derive key: %w", err)
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, fmt.Errorf("applied snapshot: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("applied snapshot: %w", err)
	}
	return &snapshotSealer{aead: aead}, nil
}

// appliedSnapshotAAD binds a row's columns to its ciphertext. The time is in
// whole seconds: databases keep different precisions.
func appliedSnapshotAAD(row *database.AppliedSnapshot) []byte {
	return []byte(appliedSnapshotInfo + "\x00" + row.Checksum + "\x00" + row.Version + "\x00" + row.Namespace + "\x00" +
		strconv.FormatInt(row.AppliedAt.UTC().Unix(), 10))
}

// snapshotSealers holds, per database, the sealer its sync services store
// applied snapshots with. A database without one stores none.
var snapshotSealers sync.Map // *gorm.Config -> *snapshotSealer

// PersistAppliedSnapshots makes every EdgeSyncService on db store the
// snapshot it applies, encrypted with a key derived from encryptionKey (the
// edge's ENCRYPTION_KEY). An edge calls it once, before its first sync.
func PersistAppliedSnapshots(db *gorm.DB, encryptionKey string) error {
	sealer, err := newSnapshotSealer(encryptionKey)
	if err != nil {
		return err
	}
	snapshotSealers.Store(db.Config, sealer)
	return nil
}

// storeAppliedSnapshot replaces the stored snapshot with raw, inside the
// transaction that applies it. skipped are the entries left out of it. It
// does nothing on a database that does not persist snapshots.
func (s *EdgeSyncService) storeAppliedSnapshot(tx *gorm.DB, raw *pb.ConfigurationSnapshot, skipped []string) error {
	v, ok := snapshotSealers.Load(s.db.Config)
	if !ok {
		return nil
	}
	sealer := v.(*snapshotSealer)
	plain, err := proto.MarshalOptions{Deterministic: true}.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	row := &database.AppliedSnapshot{
		ID:        database.AppliedSnapshotID,
		Checksum:  appliedChecksum(raw.Checksum, skipped),
		Version:   raw.Version,
		Namespace: s.namespace,
		AppliedAt: time.Now().UTC().Truncate(time.Second),
	}
	row.Snapshot = sealer.aead.Seal(nil, nil, plain, appliedSnapshotAAD(row))
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(row).Error
}

// StoredSnapshot is the snapshot an edge last applied, read back from its
// database.
type StoredSnapshot struct {
	// Checksum is the one the edge reported to the hub for it.
	Checksum  string
	Version   string
	Namespace string
	AppliedAt time.Time
	// Snapshot is the snapshot as the hub sent it.
	Snapshot *pb.ConfigurationSnapshot
}

// ErrNoStoredSnapshot is returned by LoadAppliedSnapshot when the edge has
// stored none.
var ErrNoStoredSnapshot = errors.New("no applied snapshot is stored")

// LoadAppliedSnapshot reads the stored snapshot from db and decrypts it with
// the key derived from encryptionKey. It fails with ErrNoStoredSnapshot when
// there is none, and with an error when the row does not open under this key
// (another ENCRYPTION_KEY, or a row changed in the database).
func LoadAppliedSnapshot(db *gorm.DB, encryptionKey string) (*StoredSnapshot, error) {
	sealer, err := newSnapshotSealer(encryptionKey)
	if err != nil {
		return nil, err
	}
	var row database.AppliedSnapshot
	if err := db.First(&row, database.AppliedSnapshotID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoStoredSnapshot
		}
		return nil, fmt.Errorf("read applied snapshot: %w", err)
	}
	plain, err := sealer.aead.Open(nil, nil, row.Snapshot, appliedSnapshotAAD(&row))
	if err != nil {
		return nil, fmt.Errorf("applied snapshot does not decrypt with this ENCRYPTION_KEY: %w", err)
	}
	snap := &pb.ConfigurationSnapshot{}
	if err := proto.Unmarshal(plain, snap); err != nil {
		return nil, fmt.Errorf("decode applied snapshot: %w", err)
	}
	return &StoredSnapshot{
		Checksum:  row.Checksum,
		Version:   row.Version,
		Namespace: row.Namespace,
		AppliedAt: row.AppliedAt,
		Snapshot:  snap,
	}, nil
}
