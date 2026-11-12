//go:build !enterprise

package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/pkg/credsync"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// pageStream is the server side of a SyncCredentials stream for a test.
type pageStream struct {
	grpc.ServerStream
	ctx   context.Context
	pages []*pb.CredentialSyncPage
}

func (s *pageStream) Context() context.Context { return s.ctx }
func (s *pageStream) Send(p *pb.CredentialSyncPage) error {
	s.pages = append(s.pages, p)
	return nil
}

// Credential sync is an Enterprise feature: without a registered handler
// (the Community edition) the RPC answers Unimplemented, which an edge
// takes to mean "stay on demand". A registered handler is given the call.
func TestSyncCredentials_UnimplementedWithoutAHandler(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		server, err := NewControlServer(&Config{GRPCHost: "localhost", AuthToken: "t", EncryptionKey: "0123456789abcdef0123456789abcdef"}, db)
		require.NoError(t, err)
		t.Cleanup(server.Stop)

		stream := &pageStream{ctx: context.Background()}
		err = server.SyncCredentials(&pb.CredentialSyncRequest{EdgeId: "e", HashVersion: "v1"}, stream)
		assert.Equal(t, codes.Unimplemented, status.Code(err))
		assert.Empty(t, stream.pages)

		credsync.Register(func(src credsync.Source, req *pb.CredentialSyncRequest, out grpc.ServerStreamingServer[pb.CredentialSyncPage]) error {
			assert.Same(t, server, src)
			return out.Send(&pb.CredentialSyncPage{Last: true, HashVersion: req.HashVersion})
		})
		t.Cleanup(func() { credsync.Register(nil) })
		require.NoError(t, server.SyncCredentials(&pb.CredentialSyncRequest{EdgeId: "e", HashVersion: "v1"}, stream))
		require.Len(t, stream.pages, 1)
		assert.True(t, stream.pages[0].Last)

		credsync.Register(nil)
		assert.Equal(t, codes.Unimplemented, status.Code(server.SyncCredentials(&pb.CredentialSyncRequest{}, stream)))
	})
}

// The namespace a handler serves is the registered edge's, whatever the
// request names; an empty edge ID and one with no registration are not
// registered, so the handler sends them nothing.
func TestRegisteredNamespace(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		server, err := NewControlServer(&Config{GRPCHost: "localhost", AuthToken: "t", EncryptionKey: "0123456789abcdef0123456789abcdef"}, db)
		require.NoError(t, err)
		t.Cleanup(server.Stop)
		require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-a", Namespace: "ns-a", Status: models.EdgeStatusConnected}).Error)

		ns, registered, err := server.RegisteredNamespace("edge-a")
		require.NoError(t, err)
		assert.True(t, registered)
		assert.Equal(t, "ns-a", ns)
		for _, id := range []string{"", "never-registered"} {
			ns, registered, err := server.RegisteredNamespace(id)
			require.NoError(t, err)
			assert.False(t, registered, "edge %q", id)
			assert.Empty(t, ns)
		}
	})
}

// What a handler reads: credentials by id in pages, soft-deleted ones
// included, each with its live App loaded as ValidateToken loads it; and,
// with a time, only those whose credential or App changed at or after it.
func TestCredentialSyncRows_PostgresAndSQLite(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", "credential-sync-rows-key")
		server, err := NewControlServer(&Config{GRPCHost: "localhost", AuthToken: "t", EncryptionKey: "0123456789abcdef0123456789abcdef"}, db)
		require.NoError(t, err)
		t.Cleanup(server.Stop)
		ctx := context.Background()
		owner := models.User{Email: "rows@example.com", Name: "Rows"}
		require.NoError(t, db.Create(&owner).Error)
		llm := models.LLM{Name: "rows-llm", Vendor: "openai", Active: true}
		require.NoError(t, db.Create(&llm).Error)

		old := time.Now().Add(-time.Hour)
		mk := func(name string) (*models.Credential, *models.App) {
			cred := &models.Credential{KeyID: "key-" + name, Secret: "secret-of-" + name, Active: true}
			require.NoError(t, cred.Create(db))
			app := &models.App{Name: name, UserID: owner.ID, CredentialID: cred.ID, IsActive: true}
			require.NoError(t, db.Create(app).Error)
			require.NoError(t, app.AddLLM(db, &llm))
			require.NoError(t, db.Model(&models.App{}).Where("id = ?", app.ID).UpdateColumn("updated_at", old).Error)
			require.NoError(t, db.Model(&models.Credential{}).Where("id = ?", cred.ID).UpdateColumn("updated_at", old).Error)
			return cred, app
		}
		c1, a1 := mk("one")
		c2, _ := mk("two")
		c3, a3 := mk("three")
		c4, a4 := mk("four")
		orphan := &models.Credential{KeyID: "orphan", Secret: "secret-of-orphan"}
		require.NoError(t, orphan.Create(db))
		require.NoError(t, db.Model(&models.Credential{}).Where("id = ?", orphan.ID).UpdateColumn("updated_at", old).Error)

		// Everything, two at a time.
		var all []credsync.Row
		var after uint
		for {
			page, err := server.CredentialSyncRows(ctx, after, 2, nil)
			require.NoError(t, err)
			if len(page) == 0 {
				break
			}
			require.LessOrEqual(t, len(page), 2)
			all = append(all, page...)
			after = page[len(page)-1].Credential.ID
		}
		require.Len(t, all, 5)
		assert.Equal(t, "secret-of-one", all[0].Credential.Secret, "the secret is read in plaintext, to be hashed for edges")
		assert.True(t, all[0].Hashable())
		require.NotNil(t, all[0].App)
		assert.Equal(t, a1.ID, all[0].App.ID)
		require.Len(t, all[0].App.LLMs, 1, "the App's LLMs are loaded for its AppConfig")
		assert.Nil(t, all[4].App, "a credential no App uses")
		require.NotNil(t, server.AppConfig(all[0].App))

		// Changes: a deactivated key, a deleted key, a deleted App, a
		// deactivated App. Credential two is untouched.
		cutoff := time.Now().Add(-time.Minute)
		require.NoError(t, c1.Deactivate(db))
		require.NoError(t, db.Delete(&models.Credential{}, c2.ID).Error)
		require.NoError(t, db.Delete(&models.App{}, a3.ID).Error)
		require.NoError(t, db.Model(&models.App{}).Where("id = ?", a4.ID).Update("is_active", false).Error)

		changed, err := server.CredentialSyncRows(ctx, 0, 100, &cutoff)
		require.NoError(t, err)
		byID := map[uint]credsync.Row{}
		for _, r := range changed {
			byID[r.Credential.ID] = r
		}
		require.Len(t, changed, 4, "the orphan did not change")
		assert.False(t, byID[c1.ID].Credential.Active)
		assert.True(t, byID[c2.ID].Credential.DeletedAt.Valid, "a soft-deleted credential is read, as deleted")
		assert.Nil(t, byID[c3.ID].App, "a deleted App is no App; its credential comes up because the App changed")
		require.NotNil(t, byID[c4.ID].App)
		assert.False(t, byID[c4.ID].App.IsActive)

		future := time.Now().Add(time.Minute)
		none, err := server.CredentialSyncRows(ctx, 0, 100, &future)
		require.NoError(t, err)
		assert.Empty(t, none)

		// The hash edges hold, under the key they share with the hub.
		hasher, err := server.EdgeCredentialHasher()
		require.NoError(t, err)
		want, err := credhash.New("0123456789abcdef0123456789abcdef")
		require.NoError(t, err)
		assert.Equal(t, want.Hash("secret-of-one"), hasher.Hash("secret-of-one"))
	})
}
