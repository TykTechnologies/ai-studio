package analytics

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Edge cases of the MCP statistics, run on SQLite and on Postgres
// (DATABASE_URL): LIKE metacharacters in the search, buckets across
// midnight UTC with Gateway (nanosecond) timestamps, and a larger random
// seed checked against figures computed here.

func mcpEdgeSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	return db
}

type mcpEdgeSeed struct {
	conn   uint
	server uint
	apps   []uint
}

func seedMCPEdgeBase(t *testing.T, db *gorm.DB, apps int) mcpEdgeSeed {
	t.Helper()
	require.NoError(t, models.InitModels(db))
	require.NoError(t, Migrate(db))
	owner := models.NewUser()
	owner.Email, owner.Name, owner.Password = "owner@tyk.io", "Owner", "hash"
	require.NoError(t, owner.Create(db))
	conn := &models.TykConnection{Name: "gw", DashboardURL: "http://gw:8080", Status: models.TykConnectionActive}
	require.NoError(t, db.Create(conn).Error)
	srv := &models.MCPServer{ConnectionID: &conn.ID, TykAPIID: "api-edge", Name: "Edge", Slug: "edge"}
	require.NoError(t, db.Create(srv).Error)
	s := mcpEdgeSeed{conn: conn.ID, server: srv.ID}
	for i := 0; i < apps; i++ {
		a := &models.App{Name: fmt.Sprintf("App %02d", i), UserID: owner.ID}
		require.NoError(t, a.Create(db))
		s.apps = append(s.apps, a.ID)
	}
	return s
}

func (s mcpEdgeSeed) call(name string, ts time.Time, lat int64, code int, app *uint) models.MCPCallRecord {
	sid := s.server
	return models.MCPCallRecord{Timestamp: ts, ConnectionID: s.conn, MCPServerID: &sid, AppID: app, PrimitiveType: "tool",
		PrimitiveName: name, LatencyTotalMs: lat, ResponseCode: code, Source: models.MCPCallSourceTykPump}
}

func runMCPStatsEdgeSuite(t *testing.T, open func(t *testing.T) *gorm.DB) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	dayFilter := func(s mcpEdgeSeed, from, to time.Time) MCPStatsFilter {
		return MCPStatsFilter{Start: from, End: to, ServerID: &s.server}
	}

	t.Run("search treats _ % and backslash literally", func(t *testing.T) {
		db := open(t)
		s := seedMCPEdgeBase(t, db, 0)
		ts := day.Add(time.Hour)
		for _, name := range []string{"get_forecast", "getXforecast", "100%_done", "1000_done", `a\b`, "ab"} {
			require.NoError(t, db.Create(&[]models.MCPCallRecord{s.call(name, ts, 1, 200, nil)}).Error)
		}
		f := dayFilter(s, day, day.Add(24*time.Hour-time.Second))
		for search, want := range map[string][]string{
			"t_f":   {"get_forecast"},
			"0%_":   {"100%_done"},
			"_":     {"get_forecast", "100%_done", "1000_done"},
			`\`:     {`a\b`},
			`a\b`:   {`a\b`},
			"GET_F": {"get_forecast"},
		} {
			rows, total, err := GetMCPToolStats(db, f, 1, 100, search)
			require.NoError(t, err, search)
			var got []string
			for _, r := range rows {
				got = append(got, r.PrimitiveName)
			}
			assert.ElementsMatch(t, want, got, "search %q", search)
			assert.Equal(t, int64(len(want)), total, "search %q", search)
		}
	})

	t.Run("buckets across midnight UTC with nanosecond timestamps", func(t *testing.T) {
		db := open(t)
		s := seedMCPEdgeBase(t, db, 0)
		late := day.Add(23*time.Hour + 30*time.Minute + 123456789)
		lastSecond := day.Add(24*time.Hour - time.Second + 500*time.Millisecond) // 23:59:59.5
		early := day.Add(24*time.Hour + 15*time.Minute + 987654321)
		require.NoError(t, db.Create(&[]models.MCPCallRecord{
			s.call("t", late, 10, 200, nil),
			s.call("t", lastSecond, 20, 500, nil),
			s.call("t", early, 30, 200, nil),
		}).Error)

		two := dayFilter(s, day, day.Add(48*time.Hour-time.Second))
		ts, err := GetMCPTimeseries(db, two, "hour")
		require.NoError(t, err)
		require.Len(t, ts.Points, 2)
		assert.True(t, ts.Points[0].Bucket.Equal(day.Add(23*time.Hour)), ts.Points[0].Bucket)
		assert.Equal(t, int64(2), ts.Points[0].Calls)
		assert.Equal(t, int64(1), ts.Points[0].Errors)
		assert.True(t, ts.Points[1].Bucket.Equal(day.Add(24*time.Hour)), ts.Points[1].Bucket)
		assert.Equal(t, int64(1), ts.Points[1].Calls)

		ts, err = GetMCPTimeseries(db, two, "day")
		require.NoError(t, err)
		require.Len(t, ts.Points, 2)
		assert.True(t, ts.Points[0].Bucket.Equal(day))
		assert.Equal(t, int64(2), ts.Points[0].Calls)
		assert.True(t, ts.Points[1].Bucket.Equal(day.Add(24*time.Hour)))

		// The second day alone does not see the first day's calls.
		sum, err := GetMCPSummary(db, dayFilter(s, day.Add(24*time.Hour), day.Add(48*time.Hour-time.Second)))
		require.NoError(t, err)
		assert.Equal(t, int64(1), sum.Calls)
	})

	t.Run("larger random seed matches figures computed here", func(t *testing.T) {
		db := open(t)
		s := seedMCPEdgeBase(t, db, 5)
		rnd := rand.New(rand.NewSource(42))
		type agg struct {
			lats   []float64
			errors int64
		}
		byTool := map[string]*agg{}
		byApp := map[string]*agg{}
		var rows []models.MCPCallRecord
		for i := 0; i < 2000; i++ {
			name := fmt.Sprintf("tool_%02d", rnd.Intn(30))
			lat := int64(rnd.Intn(500))
			code := 200
			if rnd.Intn(10) == 0 {
				code = 500
			}
			var app *uint
			key := "none"
			if k := rnd.Intn(6); k < 5 {
				app = &s.apps[k]
				key = fmt.Sprint(s.apps[k])
			}
			ts := day.Add(time.Duration(rnd.Int63n(int64(23 * time.Hour))))
			rows = append(rows, s.call(name, ts, lat, code, app))
			for _, mk := range []struct {
				m map[string]*agg
				k string
			}{{byTool, name}, {byApp, key}} {
				if mk.m[mk.k] == nil {
					mk.m[mk.k] = &agg{}
				}
				mk.m[mk.k].lats = append(mk.m[mk.k].lats, float64(lat))
				if code >= 400 {
					mk.m[mk.k].errors++
				}
			}
		}
		require.NoError(t, db.CreateInBatches(rows, 200).Error)
		f := dayFilter(s, day, day.Add(24*time.Hour-time.Second))

		got := map[string]MCPToolRow{}
		var total int64
		for page := 1; ; page++ {
			rs, tot, err := GetMCPToolStats(db, f, page, 7, "")
			require.NoError(t, err)
			total = tot
			for _, r := range rs {
				got[r.PrimitiveName] = r
			}
			if len(rs) < 7 {
				break
			}
		}
		assert.Equal(t, int64(len(byTool)), total)
		require.Len(t, got, len(byTool), "paging returned every tool once")
		for name, a := range byTool {
			r := got[name]
			assert.Equal(t, int64(len(a.lats)), r.Calls, name)
			assert.Equal(t, a.errors, r.Errors, name)
			assert.InDelta(t, mean(a.lats), r.AvgMs, 1e-6, name)
			assert.InDelta(t, percentileCont(a.lats, 0.5), r.P50Ms, 1e-6, name)
			assert.InDelta(t, percentileCont(a.lats, 0.95), r.P95Ms, 1e-6, name)
		}

		apps, appTotal, err := GetMCPAppStats(db, f, 1, 100, "")
		require.NoError(t, err)
		assert.Equal(t, int64(len(byApp)), appTotal)
		for _, r := range apps {
			key := "none"
			if r.AppID != nil {
				key = fmt.Sprint(*r.AppID)
			}
			a := byApp[key]
			require.NotNil(t, a, key)
			assert.Equal(t, int64(len(a.lats)), r.Calls, key)
			assert.Equal(t, a.errors, r.Errors, key)
		}

		sum, err := GetMCPSummary(db, f)
		require.NoError(t, err)
		assert.Equal(t, int64(2000), sum.Calls)
	})
}

func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// percentileCont is Postgres percentile_cont: linear interpolation at
// position p*(n-1) of the sorted values.
func percentileCont(v []float64, p float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	pos := p * float64(len(s)-1)
	lo := math.Floor(pos)
	hi := math.Ceil(pos)
	return s[int(lo)] + (s[int(hi)]-s[int(lo)])*(pos-lo)
}

func TestMCPStatsEdges_SQLite(t *testing.T) {
	runMCPStatsEdgeSuite(t, mcpEdgeSQLite)
}

func TestMCPStatsEdges_Postgres(t *testing.T) {
	runMCPStatsEdgeSuite(t, postgresSchemaDB)
}
