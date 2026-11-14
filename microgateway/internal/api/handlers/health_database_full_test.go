package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A gateway whose database has no room left is still ready: it serves from
// memory and from what is stored, and taking it out of service would turn a
// degraded gateway into an outage. /ready says so in its body: that the
// database is full, since when, and what is no longer stored.
func TestReadinessReportsAFullDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sc := &services.ServiceContainer{DB: db}
	ready := func() (int, map[string]interface{}) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
		ReadinessCheck(sc)(c)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w.Code, body
	}
	t.Cleanup(func() {
		_ = database.NoteWrite(database.WriteConfiguration, nil)
		_ = database.NoteWrite(database.WriteControlContact, nil)
	})

	code, body := ready()
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["database_full"])
	assert.NotContains(t, body, "database_full_since")

	full := errors.New("database or disk is full")
	_ = database.NoteWrite(database.WriteControlContact, full)
	_ = database.NoteWrite(database.WriteConfiguration, full)
	code, body = ready()
	require.Equal(t, http.StatusOK, code, "still ready: %v", body)
	assert.Equal(t, "ready", body["status"])
	assert.Equal(t, true, body["database_full"])
	assert.NotEmpty(t, body["database_full_since"])
	assert.Equal(t, []interface{}{"configuration", "control_contact"}, body["database_full_not_stored"])

	_ = database.NoteWrite(database.WriteConfiguration, nil)
	_ = database.NoteWrite(database.WriteControlContact, nil)
	_, body = ready()
	assert.Equal(t, false, body["database_full"])
}
