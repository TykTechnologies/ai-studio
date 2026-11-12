// Package credsync is what the control server's SyncCredentials RPC
// (EDGE_CREDENTIAL_SYNC=full on an edge) is built from in every edition:
// the hook its handler registers through, what a handler reads from the
// control server (Source), and the read of credential rows with their
// Apps.
//
// SyncCredentials streams the App credentials an edge's namespace may
// serve, as hashes, so the edge can validate keys without asking the hub.
// It is an Enterprise feature: the handler is registered by the Enterprise
// module (Register), and without one, in the Community edition, the RPC
// answers Unimplemented, which an edge takes to mean "stay on demand".
//
// The package is separate from the control server (package grpc) so that
// the Enterprise handler depends on this, the proto and the models, and
// not on the control server and everything it imports. Like every
// streaming RPC of the control server, SyncCredentials runs behind its
// stream authentication: a handler is only reached by a caller that
// presented the edge token.
package credsync

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Source is what a handler reads from the control server it serves for.
type Source interface {
	// RegisteredNamespace is the namespace an edge's requests are
	// answered for: the one the edge registered in, whatever the request
	// names; requested is used for an edge that is not registered.
	RegisteredNamespace(edgeID, requested string) string
	// NamespaceLicensed reports whether the namespace holds a licensed
	// slot (or is the default namespace).
	NamespaceLicensed(namespace string) bool
	// ServesNamespace reports whether an App of appNamespace is served by
	// edges of edgeNamespace.
	ServesNamespace(appNamespace, edgeNamespace string) bool
	// AppConfig is an App as an edge receives it for a credential. app
	// needs what Rows loads.
	AppConfig(app *models.App) *pb.AppConfig
	// NodeID is the replica's cluster node ID.
	NodeID() string
	// DatabaseFailovers is how many times the replica has seen the
	// writable database server change since it started.
	DatabaseFailovers() int64
	// DatabaseTime is the database server's clock.
	DatabaseTime(ctx context.Context) (time.Time, error)
	// EdgeCredentialHasher makes the hash an edge holds for a credential,
	// or fails with ErrNoEdgeEncryptionKey.
	EdgeCredentialHasher() (*credhash.Hasher, error)
	// CredentialSyncRows is Rows on the control server's database.
	CredentialSyncRows(ctx context.Context, afterID uint, limit int, changedSince *time.Time) ([]Row, error)
}

// Handler serves one SyncCredentials call for the control server src.
type Handler func(src Source, req *pb.CredentialSyncRequest, stream grpc.ServerStreamingServer[pb.CredentialSyncPage]) error

var handler atomic.Pointer[Handler]

// Register installs the handler of SyncCredentials for every control server
// in the process. Nil removes it.
func Register(h Handler) {
	if h == nil {
		handler.Store(nil)
		return
	}
	handler.Store(&h)
}

// Registered returns the installed handler, or nil.
func Registered() Handler {
	if h := handler.Load(); h != nil {
		return *h
	}
	return nil
}

// ErrNoEdgeEncryptionKey: the control server has no usable
// MICROGATEWAY_ENCRYPTION_KEY, so it cannot make the hashes edges hold.
var ErrNoEdgeEncryptionKey = errors.New("MICROGATEWAY_ENCRYPTION_KEY is not set or too short to hash credentials for edges")

// Row is one credential and the App that uses it.
type Row struct {
	// Credential, soft-deleted ones included (DeletedAt is then set). Its
	// Secret is the plaintext, or secrets.RefusedCredential when it does
	// not decrypt on this hub (Hashable is then false).
	Credential models.Credential
	// App is the live (not soft-deleted) App whose key this is, with what
	// AppConfig needs loaded, or nil when there is none.
	App *models.App
}

// Hashable reports whether the credential's secret can be hashed for edges.
func (r Row) Hashable() bool {
	return r.Credential.Secret != "" && r.Credential.Secret != secrets.RefusedCredential
}

// Rows reads the next page of credentials by id, after
// afterID, at most limit of them, with their Apps. With changedSince nil it
// reads every credential (soft-deleted ones included); otherwise only those
// that changed at or after that time: the credential's or its App's
// updated_at or deleted_at. The App counts whether or not it is soft-deleted,
// so that deleting or deactivating an App brings its credential up again.
//
// It reads through the models, so secrets come back decrypted, and it is
// the same on SQLite and Postgres. A page is bounded by limit: a caller
// streaming every credential holds one page at a time.
func Rows(ctx context.Context, db *gorm.DB, afterID uint, limit int, changedSince *time.Time) ([]Row, error) {
	db = db.WithContext(ctx)
	q := db.Unscoped().Where("credentials.id > ?", afterID)
	if changedSince != nil {
		t := *changedSince
		// The Apps that changed are found once, in an uncorrelated
		// subquery, not once per credential.
		q = q.Where(`credentials.updated_at >= ? OR credentials.deleted_at >= ?
			OR credentials.id IN (SELECT apps.credential_id FROM apps WHERE apps.updated_at >= ? OR apps.deleted_at >= ?)`,
			t, t, t, t)
	}
	var creds []models.Credential
	if err := q.Order("credentials.id").Limit(limit).Find(&creds).Error; err != nil {
		return nil, err
	}
	if len(creds) == 0 {
		return nil, nil
	}

	ids := make([]uint, len(creds))
	for i, c := range creds {
		ids[i] = c.ID
	}
	var apps []models.App
	if err := db.Where("credential_id IN ?", ids).
		Preload("LLMs").Preload("Tools").Preload("Datasources").Preload("ModelRouters").Preload("SemanticRouters").
		Order("id").Find(&apps).Error; err != nil {
		return nil, err
	}
	byCredential := make(map[uint]*models.App, len(apps))
	for i := range apps {
		// The first App by id, as ValidateToken takes it.
		if _, ok := byCredential[apps[i].CredentialID]; !ok {
			byCredential[apps[i].CredentialID] = &apps[i]
		}
	}

	rows := make([]Row, len(creds))
	for i, c := range creds {
		rows[i] = Row{Credential: c, App: byCredential[c.ID]}
	}
	return rows, nil
}
