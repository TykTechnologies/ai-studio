package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The control server validates an edge's token by comparing hashes: it does
// not read, and so does not decrypt, the stored secret. A key whose stored
// secret is unreadable still validates (the hash is what identifies it), no
// key is derived to answer, and a token that is the stored form or the hash
// itself is refused.
func TestValidateToken_ComparesHashesWithoutReadingTheSecret(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", "validate-token-at-rest-key")
		server, err := NewControlServer(&Config{GRPCHost: "localhost", AuthToken: "t", EncryptionKey: "0123456789abcdef0123456789abcdef"}, db)
		require.NoError(t, err)
		t.Cleanup(server.Stop)

		owner := models.User{Email: "validate@example.com", Name: "Validate"}
		require.NoError(t, db.Create(&owner).Error)
		cred := &models.Credential{KeyID: "validate-key", Secret: "the-app-key-an-edge-presents", Active: true}
		require.NoError(t, cred.Create(db))
		app := &models.App{Name: "Validated", UserID: owner.ID, CredentialID: cred.ID, IsActive: true}
		require.NoError(t, db.Create(app).Error)

		var stored struct{ Secret, SecretHash string }
		require.NoError(t, db.Table("credentials").Select("secret", "secret_hash").Where("id = ?", cred.ID).Scan(&stored).Error)
		require.True(t, secrets.IsDataKeyValue(stored.Secret))

		validate := func(token string) *pb.TokenValidationResponse {
			resp, err := server.ValidateToken(context.Background(), &pb.TokenValidationRequest{Token: token, EdgeId: "edge-1"})
			require.NoError(t, err)
			return resp
		}
		before := secrets.KeyDerivations()
		resp := validate("the-app-key-an-edge-presents")
		assert.True(t, resp.Valid, resp.ErrorMessage)
		assert.Equal(t, uint32(app.ID), resp.AppId)
		assert.Equal(t, before, secrets.KeyDerivations(), "nothing was decrypted to validate the key")

		assert.False(t, validate(stored.Secret).Valid, "the stored form is not the key")
		assert.False(t, validate(stored.SecretHash).Valid, "nor is its hash")
		assert.False(t, validate("another-key").Valid)

		// The stored secret made unreadable: validation does not depend on it.
		require.NoError(t, db.Exec("UPDATE credentials SET secret = ? WHERE id = ?", secrets.DataKeyCurrentPrefix()+"bm90LWEtY2lwaGVydGV4dA==", cred.ID).Error)
		assert.True(t, validate("the-app-key-an-edge-presents").Valid)
	})
}
