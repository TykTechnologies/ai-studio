package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mcpStatsSeed struct {
	day      time.Time
	weather  uint
	other    uint
	appAlpha uint
	appBeta  uint
}

// seedMCPStats writes a known set of calls. On the weather server:
//
//	get_forecast: latencies 10 20 30 40 100; one HTTP 500, one JSON-RPC
//	              error in an HTTP 200; App Alpha x3, unattributed x2
//	list_cities:  latencies 5 15; App Beta x2
//
// One call to the other server (App Alpha), and one weather call a day
// before the range.
func seedMCPStats(t *testing.T, db *gorm.DB) mcpStatsSeed {
	t.Helper()
	require.NoError(t, models.InitModels(db))
	require.NoError(t, Migrate(db))

	owner := models.NewUser()
	owner.Email, owner.Name, owner.Password = "owner@tyk.io", "Owner", "hash"
	require.NoError(t, owner.Create(db))
	alpha := &models.App{Name: "Alpha App", UserID: owner.ID}
	require.NoError(t, alpha.Create(db))
	beta := &models.App{Name: "Beta App", UserID: owner.ID}
	require.NoError(t, beta.Create(db))

	conn := &models.TykConnection{Name: "dash", DashboardURL: "https://dash.example.com", Status: models.TykConnectionActive}
	require.NoError(t, db.Create(conn).Error)
	weather := &models.MCPServer{ConnectionID: &conn.ID, TykAPIID: "api-weather", Name: "Weather", Slug: "weather"}
	require.NoError(t, db.Create(weather).Error)
	other := &models.MCPServer{ConnectionID: &conn.ID, TykAPIID: "api-other", Name: "Other", Slug: "other"}
	require.NoError(t, db.Create(other).Error)

	s := mcpStatsSeed{day: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), weather: weather.ID, other: other.ID, appAlpha: alpha.ID, appBeta: beta.ID}
	at := func(h, m int) time.Time { return s.day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
	row := func(server uint, app *uint, name string, lat int64, code, rpc int, ts time.Time) models.MCPCallRecord {
		sid := server
		return models.MCPCallRecord{Timestamp: ts, ConnectionID: conn.ID, MCPServerID: &sid, AppID: app, PrimitiveType: "tool",
			PrimitiveName: name, LatencyTotalMs: lat, ResponseCode: code, JSONRPCErrorCode: rpc, Source: models.MCPCallSourceTykPump}
	}
	a, b := &s.appAlpha, &s.appBeta
	rows := []models.MCPCallRecord{
		row(s.weather, a, "get_forecast", 10, 200, 0, at(9, 0)),
		row(s.weather, a, "get_forecast", 20, 500, 0, at(9, 10)),
		row(s.weather, a, "get_forecast", 30, 200, -32602, at(9, 20)),
		row(s.weather, nil, "get_forecast", 40, 200, 0, at(10, 0)),
		row(s.weather, nil, "get_forecast", 100, 200, 0, at(10, 5)),
		row(s.weather, b, "list_cities", 5, 200, 0, at(10, 10)),
		row(s.weather, b, "list_cities", 15, 200, 0, at(10, 30)),
		row(s.other, a, "ping", 7, 200, 0, at(11, 0)),
		row(s.weather, a, "get_forecast", 9999, 500, 0, s.day.Add(-12*time.Hour)),
	}
	require.NoError(t, db.Create(&rows).Error)
	return s
}

func (s mcpStatsSeed) filter() MCPStatsFilter {
	return MCPStatsFilter{Start: s.day, End: s.day.Add(24 * time.Hour)}
}

func (s mcpStatsSeed) serverFilter() MCPStatsFilter {
	f := s.filter()
	f.ServerID = &s.weather
	return f
}

func runMCPStatsSuite(t *testing.T, db *gorm.DB) {
	s := seedMCPStats(t, db)

	t.Run("summary", func(t *testing.T) {
		sum, err := GetMCPSummary(db, s.serverFilter())
		require.NoError(t, err)
		assert.Equal(t, int64(7), sum.Calls)
		assert.Equal(t, int64(2), sum.Errors, "HTTP >= 400 or a JSON-RPC error")
		assert.InDelta(t, 2.0/7, sum.ErrorRate, 1e-9)
		assert.InDelta(t, 220.0/7, sum.AvgMs, 1e-9)
		// [5 10 15 20 30 40 100]: p50 at position 3, p95 at 5.7.
		assert.InDelta(t, 20, sum.P50Ms, 1e-9)
		assert.InDelta(t, 82, sum.P95Ms, 1e-9)
		require.NotNil(t, sum.LastSeenAt)
		assert.True(t, sum.LastSeenAt.Equal(s.day.Add(10*time.Hour+30*time.Minute)), sum.LastSeenAt)

		empty := s.serverFilter()
		empty.Start, empty.End = s.day.AddDate(0, 0, 5), s.day.AddDate(0, 0, 6)
		sum, err = GetMCPSummary(db, empty)
		require.NoError(t, err)
		assert.Zero(t, sum.Calls)
		assert.Nil(t, sum.LastSeenAt)
	})

	t.Run("timeseries", func(t *testing.T) {
		ts, err := GetMCPTimeseries(db, s.serverFilter(), "hour")
		require.NoError(t, err)
		require.Len(t, ts.Points, 2)
		assert.True(t, ts.Points[0].Bucket.Equal(s.day.Add(9*time.Hour)))
		assert.Equal(t, int64(3), ts.Points[0].Calls)
		assert.Equal(t, int64(2), ts.Points[0].Errors)
		assert.InDelta(t, 20, ts.Points[0].P50Ms, 1e-9)
		assert.InDelta(t, 29, ts.Points[0].P95Ms, 1e-9) // [10 20 30] at 1.9
		assert.Equal(t, int64(4), ts.Points[1].Calls)
		assert.Zero(t, ts.Points[1].Errors)

		ts, err = GetMCPTimeseries(db, s.serverFilter(), "day")
		require.NoError(t, err)
		require.Len(t, ts.Points, 1)
		assert.True(t, ts.Points[0].Bucket.Equal(s.day))
		assert.Equal(t, int64(7), ts.Points[0].Calls)

		_, err = GetMCPTimeseries(db, s.serverFilter(), "minute")
		assert.Error(t, err)
	})

	t.Run("tools", func(t *testing.T) {
		rows, total, err := GetMCPToolStats(db, s.serverFilter(), 1, 10, "")
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		require.Len(t, rows, 2)
		gf := rows[0]
		assert.Equal(t, "get_forecast", gf.PrimitiveName)
		assert.Equal(t, "tool", gf.PrimitiveType)
		assert.Equal(t, int64(5), gf.Calls)
		assert.Equal(t, int64(2), gf.Errors)
		assert.InDelta(t, 0.4, gf.ErrorRate, 1e-9)
		assert.InDelta(t, 40, gf.AvgMs, 1e-9)
		assert.InDelta(t, 30, gf.P50Ms, 1e-9)
		assert.InDelta(t, 88, gf.P95Ms, 1e-9) // [10 20 30 40 100] at 3.8
		lc := rows[1]
		assert.Equal(t, "list_cities", lc.PrimitiveName)
		assert.InDelta(t, 10, lc.P50Ms, 1e-9)
		assert.InDelta(t, 14.5, lc.P95Ms, 1e-9)

		// Paged and searched in SQL.
		rows, total, err = GetMCPToolStats(db, s.serverFilter(), 2, 1, "")
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		require.Len(t, rows, 1)
		assert.Equal(t, "list_cities", rows[0].PrimitiveName)
		assert.InDelta(t, 14.5, rows[0].P95Ms, 1e-9)

		rows, total, err = GetMCPToolStats(db, s.serverFilter(), 1, 10, "CITIES")
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, rows, 1)

		// LIKE wildcards in the search are literal.
		_, total, err = GetMCPToolStats(db, s.serverFilter(), 1, 10, "%")
		require.NoError(t, err)
		assert.Zero(t, total)
	})

	t.Run("apps", func(t *testing.T) {
		rows, total, err := GetMCPAppStats(db, s.serverFilter(), 1, 10, "")
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		require.Len(t, rows, 3)
		assert.Equal(t, s.appAlpha, *rows[0].AppID)
		assert.Equal(t, "Alpha App", rows[0].AppName)
		assert.Equal(t, int64(3), rows[0].Calls)
		assert.Equal(t, int64(2), rows[0].Errors)
		var unattributed *MCPAppRow
		for i := range rows {
			if rows[i].AppID == nil {
				unattributed = &rows[i]
			}
		}
		require.NotNil(t, unattributed)
		assert.Equal(t, int64(2), unattributed.Calls)

		rows, total, err = GetMCPAppStats(db, s.serverFilter(), 1, 10, "beta")
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, rows, 1)
		assert.Equal(t, s.appBeta, *rows[0].AppID)
	})

	t.Run("app servers", func(t *testing.T) {
		f := s.filter()
		f.AppID = &s.appAlpha
		rows, err := GetMCPServerStats(db, f)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, s.weather, rows[0].MCPServerID)
		assert.Equal(t, "Weather", rows[0].ServerName)
		assert.Equal(t, int64(3), rows[0].Calls)
		assert.InDelta(t, 29, rows[0].P95Ms, 1e-9)
		assert.Equal(t, s.other, rows[1].MCPServerID)
		assert.Equal(t, int64(1), rows[1].Calls)
		assert.InDelta(t, 7, rows[1].P95Ms, 1e-9)

		// Beta never called the other server.
		f.AppID = &s.appBeta
		rows, err = GetMCPServerStats(db, f)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, s.weather, rows[0].MCPServerID)
	})

	t.Run("overview", func(t *testing.T) {
		o, err := GetMCPOverview(db, s.filter())
		require.NoError(t, err)
		assert.Equal(t, int64(8), o.Calls)
		assert.Equal(t, int64(2), o.Errors)
		require.Len(t, o.TopTools, 3)
		assert.Equal(t, "get_forecast", o.TopTools[0].PrimitiveName)
		assert.Equal(t, "Weather", o.TopTools[0].ServerName)
		assert.Equal(t, int64(5), o.TopTools[0].Calls)
	})
}

func TestMCPStats_SQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	runMCPStatsSuite(t, db)
}

// Percentiles come from percentile_cont on Postgres and must match the
// SQLite emulation (DATABASE_URL, as CI's Postgres job sets it).
func TestMCPStats_Postgres(t *testing.T) {
	runMCPStatsSuite(t, postgresSchemaDB(t))
}
