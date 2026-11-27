package compliance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// withEnterprise makes IsEnterpriseAvailable true for the test (the
// enterprise build registers the real factory; EventsByRequest needs none).
func withEnterprise(t *testing.T) {
	t.Helper()
	prev := enterpriseFactory
	if prev == nil {
		enterpriseFactory = func(*gorm.DB) Service { return newCommunityService() }
	}
	t.Cleanup(func() { enterpriseFactory = prev })
}

func eventsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.App{}, &models.ComplianceEvent{}))
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			s.Close()
		}
	})
	return db
}

func addEvent(t *testing.T, db *gorm.DB, app uint, requestID string, at time.Time, eventType string) uint {
	t.Helper()
	meta := `{"action":"block","detector":"secrets.aws"}`
	if requestID != "" {
		meta = fmt.Sprintf(`{"action":"block","detector":"secrets.aws","request_id":%q}`, requestID)
	}
	e := models.ComplianceEvent{AppID: app, FilterName: "no secrets", EventType: eventType, Severity: "critical", Metadata: meta, TimeStamp: at}
	require.NoError(t, db.Create(&e).Error)
	return e.ID
}

// tkg.2: a trace's guardrail outcome finds its compliance events by the
// gateway request id, within the App and a window around the request.
func TestEventsByRequest(t *testing.T) {
	withEnterprise(t)
	db := eventsDB(t)
	require.NoError(t, db.Create(&models.App{Name: "support bot"}).Error) // id 1
	const id = "tyai_0123456789abcdef0123456789abcdef"
	const other = "tyai_ffffffffffffffffffffffffffffffff"
	at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

	first := addEvent(t, db, 1, id, at.Add(time.Second), "guardrail.secrets.aws")
	second := addEvent(t, db, 1, id, at.Add(2*time.Second), "guardrail.judge")
	addEvent(t, db, 1, other, at, "guardrail.secrets.aws")                           // another request
	addEvent(t, db, 2, id, at, "guardrail.secrets.aws")                              // another App
	addEvent(t, db, 1, id, at.Add(RequestEventWindow+time.Minute), "guardrail.late") // outside the window
	addEvent(t, db, 1, "", at, "guardrail.secrets.aws")                              // no request id

	got, err := EventsByRequest(db, 1, id, at)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []uint{first, second}, []uint{got[0].ID, got[1].ID}, "oldest first")
	assert.Equal(t, "support bot", got[0].AppName)
	assert.Equal(t, id, got[0].Metadata["request_id"])

	got, err = EventsByRequest(db, 1, "tyai_00000000000000000000000000000000", at)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got, "an empty list, not null")

	for i := 0; i < RequestEventLimit+5; i++ {
		addEvent(t, db, 1, other, at.Add(time.Duration(i)*time.Millisecond), "guardrail.many")
	}
	got, err = EventsByRequest(db, 1, other, at)
	require.NoError(t, err)
	assert.Len(t, got, RequestEventLimit)

	for _, bad := range []string{"", "tyai_", "tyai_0123", `tyai_0123456789abcdef0123456789abcde%`, "x' OR '1'='1", id + "0"} {
		_, err := EventsByRequest(db, 1, bad, at)
		assert.ErrorIs(t, err, ErrInvalidRequestID, bad)
	}
}

func TestEventsByRequest_CommunityEdition(t *testing.T) {
	prev := enterpriseFactory
	enterpriseFactory = nil
	t.Cleanup(func() { enterpriseFactory = prev })
	_, err := EventsByRequest(eventsDB(t), 1, "tyai_0123456789abcdef0123456789abcdef", time.Now())
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}
