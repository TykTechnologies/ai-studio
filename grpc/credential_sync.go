package grpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/pkg/credsync"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The control server's side of SyncCredentials (see pkg/credsync): the RPC,
// which hands the call to the registered handler, and what a handler reads
// from this server (credsync.Source).

// SyncCredentials implements the RPC: the handler registered with
// credsync.Register, or Unimplemented.
func (s *ControlServer) SyncCredentials(req *pb.CredentialSyncRequest, stream grpc.ServerStreamingServer[pb.CredentialSyncPage]) error {
	h := credsync.Registered()
	if h == nil {
		return status.Error(codes.Unimplemented, "credential sync is not available on this control plane")
	}
	return h(s, req, stream)
}

var _ credsync.Source = (*ControlServer)(nil)

// RegisteredNamespace is the namespace an edge's requests are answered for:
// the one the edge registered in, whatever the request names, as for
// ValidateToken and the edge's configuration. registered is false for an
// empty edge ID and for one with no registration; err is a failed lookup.
func (s *ControlServer) RegisteredNamespace(edgeID string) (namespace string, registered bool, err error) {
	if edgeID == "" {
		return "", false, nil
	}
	var edge models.EdgeInstance
	switch err := edge.GetByEdgeID(s.db, edgeID); {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return edge.Namespace, true, nil
}

// NamespaceLicensed reports whether ns holds a licensed slot (or is the
// default namespace). Edges of an unlicensed namespace get no credentials,
// as they get no configuration and validate no tokens.
func (s *ControlServer) NamespaceLicensed(ns string) bool { return s.namespaceLicensed(ns) }

// ServesNamespace reports whether an App of appNamespace is served by edges
// of edgeNamespace: a global App by every edge, a namespaced one by its own.
func (s *ControlServer) ServesNamespace(appNamespace, edgeNamespace string) bool {
	return s.shouldIncludeAppInResponse(appNamespace, edgeNamespace)
}

// AppConfig is an App as an edge receives it for a credential (the form
// ValidateToken returns for pull-on-miss). app needs its LLMs, Tools,
// Datasources and routers loaded, as credsync.Rows loads them.
func (s *ControlServer) AppConfig(app *models.App) *pb.AppConfig { return s.convertAppToProto(app) }

// NodeID is this replica's cluster node ID.
func (s *ControlServer) NodeID() string { return s.nodeID }

// DatabaseFailovers is how many times this replica has seen the writable
// database server change since it started (0 for SQLite and for a pool the
// host opened itself).
func (s *ControlServer) DatabaseFailovers() int64 {
	return dbstate.For(s.db).Status().Failovers
}

// DatabaseTime is the database server's clock. SQLite is in this process:
// its clock is this process's.
func (s *ControlServer) DatabaseTime(ctx context.Context) (time.Time, error) {
	if s.db.Dialector.Name() != "postgres" {
		return time.Now(), nil
	}
	var now time.Time
	err := s.db.WithContext(ctx).Raw("SELECT now()").Row().Scan(&now)
	return now, err
}

// EdgeCredentialHasher makes the hash an edge holds for a credential:
// pkg/credhash keyed with MICROGATEWAY_ENCRYPTION_KEY, which is each edge's
// ENCRYPTION_KEY, so hub and edge compute the same hash. It is not the
// hub's own lookup hash (secrets.LookupHash, keyed with TYK_AI_SECRET_KEY):
// rotating the edge key changes what edges are sent, not what the hub
// stores.
func (s *ControlServer) EdgeCredentialHasher() (*credhash.Hasher, error) {
	h, err := credhash.New(s.encryptionKey)
	if err != nil {
		return nil, credsync.ErrNoEdgeEncryptionKey
	}
	return h, nil
}

// CredentialSyncRows reads a page of credentials with their Apps from this
// server's database (credsync.Rows).
func (s *ControlServer) CredentialSyncRows(ctx context.Context, afterID uint, limit int, changedSince *time.Time) ([]credsync.Row, error) {
	return credsync.Rows(ctx, s.db, afterID, limit, changedSince)
}
