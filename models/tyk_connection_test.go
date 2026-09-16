package models

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func tykTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&TykConnection{}))
	return db
}

func TestTykConnection_RefusesToStoreTokenWithoutKey(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	db := tykTestDB(t)
	conn := &TykConnection{Name: "dash", DashboardURL: "https://dash.example.com", DashboardAccessToken: "plain-token-value"}
	err := db.Create(conn).Error
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretsKeyRequired)

	var count int64
	db.Model(&TykConnection{}).Count(&count)
	assert.Equal(t, int64(0), count, "nothing may be written without the key")
}

func TestTykConnection_EncryptsTokensAtRest(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "unit-test-key")
	db := tykTestDB(t)
	conn := &TykConnection{
		Name: "dash", DashboardURL: "https://dash.example.com",
		DashboardAccessToken: "dash-token-1234", MDCBURL: "https://mdcb.example.com", MDCBAccessToken: "mdcb-secret-5678",
	}
	require.NoError(t, db.Create(conn).Error)
	// In-memory struct keeps plaintext for callers.
	assert.Equal(t, "dash-token-1234", conn.DashboardAccessToken)

	var raw map[string]interface{}
	require.NoError(t, db.Table("tyk_connections").Where("id = ?", conn.ID).Take(&raw).Error)
	stored, _ := raw["dashboard_access_token"].(string)
	assert.True(t, strings.HasPrefix(stored, "$ENC/"), "token must be ciphertext at rest, got %q", stored)
	assert.NotContains(t, stored, "dash-token-1234")
	mdcb, _ := raw["mdcb_access_token"].(string)
	assert.True(t, strings.HasPrefix(mdcb, "$ENC/"))

	var loaded TykConnection
	require.NoError(t, db.First(&loaded, conn.ID).Error)
	assert.Equal(t, "dash-token-1234", loaded.DashboardAccessToken)
	assert.Equal(t, "mdcb-secret-5678", loaded.MDCBAccessToken)

	// Saving again must not double-encrypt.
	loaded.Name = "renamed"
	require.NoError(t, db.Save(&loaded).Error)
	var again TykConnection
	require.NoError(t, db.First(&again, conn.ID).Error)
	assert.Equal(t, "dash-token-1234", again.DashboardAccessToken)
}

func TestTykConnection_ResponseNeverCarriesTokens(t *testing.T) {
	conn := &TykConnection{Name: "dash", DashboardAccessToken: "dash-token-1234", MDCBAccessToken: "mdcb-secret-5678"}
	conn.SetDataPlanes([]TykDataPlane{{GroupID: "eu", Tags: []string{"edge-eu"}, NodeCount: 2}})
	conn.SetKnownGatewayTags([]TykGatewayTag{{Tag: "edge-us"}})
	view := conn.ToResponse()
	b, err := json.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "dash-token-1234")
	assert.NotContains(t, string(b), "mdcb-secret-5678")
	assert.True(t, view.HasToken)
	assert.Equal(t, "1234", view.TokenHint)
	assert.True(t, view.HasMDCBToken)
	assert.ElementsMatch(t, []string{"edge-eu", "edge-us"}, view.GatewayTags)

	// Direct JSON of the model must not leak either.
	mb, err := json.Marshal(conn)
	require.NoError(t, err)
	assert.NotContains(t, string(mb), "dash-token-1234")
}

func TestTykConnection_ModeRank(t *testing.T) {
	assert.Less(t, TykModeRank(TykConnectionModeCatalogue), TykModeRank(TykConnectionModeBroker))
	assert.Less(t, TykModeRank(TykConnectionModeBroker), TykModeRank(TykConnectionModeFull))
	assert.Equal(t, -1, TykModeRank("bogus"))
}
