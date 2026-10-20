package analytics

import (
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// MCP call statistics over mcp_call_records: the calls Tyk Gateways proxied
// to MCP servers Studio brokers, delivered by Tyk Pump. Everything is
// aggregated, filtered and paged in SQL. Latency percentiles are
// interpolated (percentile_cont): natively on Postgres, and with window
// functions on SQLite, so both engines give the same numbers.

// mcpErrorExpr is 1 for a failed call: an HTTP error, or a JSON-RPC error in
// a successful HTTP response.
const mcpErrorExpr = "CASE WHEN response_code >= 400 OR jsonrpc_error_code <> 0 THEN 1 ELSE 0 END"

// MCPStatsFilter selects the calls a statistic covers. Start and End bound
// the timestamp (inclusive); nil ids do not filter.
type MCPStatsFilter struct {
	Start    time.Time
	End      time.Time
	ServerID *uint
	AppID    *uint
}

func (f MCPStatsFilter) apply(q *gorm.DB) *gorm.DB {
	q = q.Where("mcp_call_records.timestamp BETWEEN ? AND ?", f.Start.UTC(), f.End.UTC())
	if f.ServerID != nil {
		q = q.Where("mcp_call_records.mcp_server_id = ?", *f.ServerID)
	}
	if f.AppID != nil {
		q = q.Where("mcp_call_records.app_id = ?", *f.AppID)
	}
	return q
}

// MCPSummary is the headline numbers of a set of calls.
type MCPSummary struct {
	Calls      int64      `json:"calls"`
	Errors     int64      `json:"errors"`
	ErrorRate  float64    `json:"error_rate"`
	AvgMs      float64    `json:"avg_ms"`
	P50Ms      float64    `json:"p50_ms"`
	P95Ms      float64    `json:"p95_ms"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

// MCPTimeseriesPoint is one bucket of a time series.
type MCPTimeseriesPoint struct {
	Bucket time.Time `json:"bucket"`
	Calls  int64     `json:"calls"`
	Errors int64     `json:"errors"`
	P50Ms  float64   `json:"p50_ms"`
	P95Ms  float64   `json:"p95_ms"`
}

// MCPTimeseries is calls and errors over time.
type MCPTimeseries struct {
	Interval string               `json:"interval"`
	Points   []MCPTimeseriesPoint `json:"points"`
}

// MCPToolRow is one tool (or resource, prompt...) of a server.
type MCPToolRow struct {
	PrimitiveType string     `json:"primitive_type"`
	PrimitiveName string     `json:"primitive_name"`
	Calls         int64      `json:"calls"`
	Errors        int64      `json:"errors"`
	ErrorRate     float64    `json:"error_rate"`
	AvgMs         float64    `json:"avg_ms"`
	P50Ms         float64    `json:"p50_ms"`
	P95Ms         float64    `json:"p95_ms"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
}

// MCPAppRow is one App's use of a server. A nil AppID is the traffic no App
// could be attributed to.
type MCPAppRow struct {
	AppID      *uint      `json:"app_id"`
	AppName    string     `json:"app_name"`
	Calls      int64      `json:"calls"`
	Errors     int64      `json:"errors"`
	ErrorRate  float64    `json:"error_rate"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

// MCPServerRow is one server's traffic (for an App, or overall).
type MCPServerRow struct {
	MCPServerID  uint       `json:"mcp_server_id"`
	ServerName   string     `json:"server_name"`
	ConnectionID uint       `json:"connection_id"`
	Calls        int64      `json:"calls"`
	Errors       int64      `json:"errors"`
	ErrorRate    float64    `json:"error_rate"`
	P95Ms        float64    `json:"p95_ms"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
}

// MCPTopTool is a tool in the overview's busiest list.
type MCPTopTool struct {
	MCPServerID   uint    `json:"mcp_server_id"`
	ServerName    string  `json:"server_name"`
	PrimitiveType string  `json:"primitive_type"`
	PrimitiveName string  `json:"primitive_name"`
	Calls         int64   `json:"calls"`
	Errors        int64   `json:"errors"`
	ErrorRate     float64 `json:"error_rate"`
}

// MCPOverview is all MCP traffic: the summary and the busiest tools.
type MCPOverview struct {
	MCPSummary
	TopTools []MCPTopTool `json:"top_tools"`
}

// MCPIntervals are the time series bucket sizes.
var MCPIntervals = map[string]bool{"hour": true, "day": true}

func rate(errors, calls int64) float64 {
	if calls == 0 {
		return 0
	}
	return float64(errors) / float64(calls)
}

func timePtr(t ScanTime) *time.Time {
	if t.Time.IsZero() {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func isPostgres(db *gorm.DB) bool {
	return db.Dialector.Name() == "postgres"
}

// aggRow is the common aggregate columns.
type aggRow struct {
	Calls    int64
	Errors   int64
	AvgMs    float64
	LastSeen ScanTime
}

const aggSelect = "COUNT(*) AS calls, COALESCE(SUM(" + mcpErrorExpr + "), 0) AS errors, " +
	"COALESCE(AVG(latency_total_ms), 0) AS avg_ms, MAX(mcp_call_records.timestamp) AS last_seen"

// GetMCPSummary returns the headline numbers of the calls f selects.
func GetMCPSummary(db *gorm.DB, f MCPStatsFilter) (*MCPSummary, error) {
	var a aggRow
	if err := f.apply(db.Model(&models.MCPCallRecord{})).Select(aggSelect).Scan(&a).Error; err != nil {
		return nil, err
	}
	out := &MCPSummary{Calls: a.Calls, Errors: a.Errors, ErrorRate: rate(a.Errors, a.Calls), AvgMs: a.AvgMs, LastSeenAt: timePtr(a.LastSeen)}
	if a.Calls == 0 {
		return out, nil
	}
	pct, err := mcpPercentiles(db, f, "", nil)
	if err != nil {
		return nil, err
	}
	p := pct[""]
	out.P50Ms, out.P95Ms = p.p50, p.p95
	return out, nil
}

// bucketExpr renders the start of a call's bucket as an RFC 3339 UTC string.
func bucketExpr(db *gorm.DB, interval string) string {
	if isPostgres(db) {
		if interval == "hour" {
			return `to_char(date_trunc('hour', mcp_call_records.timestamp AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"HH24:00:00"Z"')`
		}
		return `to_char(date_trunc('day', mcp_call_records.timestamp AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"00:00:00"Z"')`
	}
	if interval == "hour" {
		return `strftime('%Y-%m-%dT%H:00:00Z', mcp_call_records.timestamp)`
	}
	return `strftime('%Y-%m-%dT00:00:00Z', mcp_call_records.timestamp)`
}

// GetMCPTimeseries returns calls, errors and latency per bucket ("hour" or
// "day"). Only buckets with calls are returned.
func GetMCPTimeseries(db *gorm.DB, f MCPStatsFilter, interval string) (*MCPTimeseries, error) {
	if !MCPIntervals[interval] {
		return nil, fmt.Errorf("interval must be hour or day, got %q", interval)
	}
	bucket := bucketExpr(db, interval)
	var rows []struct {
		Bucket string
		Calls  int64
		Errors int64
	}
	if err := f.apply(db.Model(&models.MCPCallRecord{})).
		Select(bucket + " AS bucket, COUNT(*) AS calls, COALESCE(SUM(" + mcpErrorExpr + "), 0) AS errors").
		Group(bucket).Order("bucket").Scan(&rows).Error; err != nil {
		return nil, err
	}
	pct, err := mcpPercentiles(db, f, bucket, nil)
	if err != nil {
		return nil, err
	}
	out := &MCPTimeseries{Interval: interval, Points: make([]MCPTimeseriesPoint, 0, len(rows))}
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339, r.Bucket)
		if err != nil {
			return nil, fmt.Errorf("unexpected bucket %q: %w", r.Bucket, err)
		}
		p := pct[r.Bucket]
		out.Points = append(out.Points, MCPTimeseriesPoint{Bucket: t, Calls: r.Calls, Errors: r.Errors, P50Ms: p.p50, P95Ms: p.p95})
	}
	return out, nil
}

// likePattern escapes s for a LIKE ... ESCAPE '\' substring match.
func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + strings.ToLower(r.Replace(s)) + "%"
}

func pageBounds(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}

// toolKeyExpr identifies a tool within a server.
const toolKeyExpr = "COALESCE(primitive_type, '') || ':' || COALESCE(primitive_name, '')"

// GetMCPToolStats returns one page of a server's tools (every primitive:
// tools, resources, prompts, discovery), busiest first. search matches the
// primitive name.
func GetMCPToolStats(db *gorm.DB, f MCPStatsFilter, page, pageSize int, search string) ([]MCPToolRow, int64, error) {
	page, pageSize = pageBounds(page, pageSize)
	base := func() *gorm.DB {
		q := f.apply(db.Model(&models.MCPCallRecord{}))
		if search != "" {
			q = q.Where(`LOWER(primitive_name) LIKE ? ESCAPE '\'`, likePattern(search))
		}
		return q
	}
	var total int64
	if err := db.Table("(?) AS t", base().Select("primitive_type, primitive_name").Group("primitive_type, primitive_name")).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		PrimitiveType string
		PrimitiveName string
		Calls         int64
		Errors        int64
		AvgMs         float64
		LastSeen      ScanTime
	}
	if err := base().Select("COALESCE(primitive_type, '') AS primitive_type, COALESCE(primitive_name, '') AS primitive_name, " + aggSelect).
		Group("primitive_type, primitive_name").
		Order("calls DESC, primitive_name, primitive_type").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.PrimitiveType + ":" + r.PrimitiveName
	}
	pct, err := mcpPercentiles(db, f, toolKeyExpr, keys)
	if err != nil {
		return nil, 0, err
	}
	out := make([]MCPToolRow, len(rows))
	for i, r := range rows {
		p := pct[keys[i]]
		out[i] = MCPToolRow{PrimitiveType: r.PrimitiveType, PrimitiveName: r.PrimitiveName, Calls: r.Calls, Errors: r.Errors,
			ErrorRate: rate(r.Errors, r.Calls), AvgMs: r.AvgMs, P50Ms: p.p50, P95Ms: p.p95, LastSeenAt: timePtr(r.LastSeen)}
	}
	return out, total, nil
}

// GetMCPAppStats returns one page of the Apps calling a server, busiest
// first, with the unattributed traffic as a row of its own. search matches
// the App name.
func GetMCPAppStats(db *gorm.DB, f MCPStatsFilter, page, pageSize int, search string) ([]MCPAppRow, int64, error) {
	page, pageSize = pageBounds(page, pageSize)
	base := func() *gorm.DB {
		q := f.apply(db.Model(&models.MCPCallRecord{})).
			Joins("LEFT JOIN apps ON apps.id = mcp_call_records.app_id")
		if search != "" {
			q = q.Where(`LOWER(apps.name) LIKE ? ESCAPE '\'`, likePattern(search))
		}
		return q
	}
	var total int64
	if err := db.Table("(?) AS t", base().Select("mcp_call_records.app_id").Group("mcp_call_records.app_id")).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		AppID    *uint
		AppName  *string
		Calls    int64
		Errors   int64
		AvgMs    float64
		LastSeen ScanTime
	}
	if err := base().Select("mcp_call_records.app_id AS app_id, MAX(apps.name) AS app_name, " + aggSelect).
		Group("mcp_call_records.app_id").
		Order("calls DESC, app_id").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]MCPAppRow, len(rows))
	for i, r := range rows {
		name := ""
		if r.AppName != nil {
			name = *r.AppName
		}
		out[i] = MCPAppRow{AppID: r.AppID, AppName: name, Calls: r.Calls, Errors: r.Errors, ErrorRate: rate(r.Errors, r.Calls), LastSeenAt: timePtr(r.LastSeen)}
	}
	return out, total, nil
}

// maxServerRows bounds the per-server breakdown; an App or the overview
// reaching more servers than this is not a case the views page through.
const maxServerRows = 200

// GetMCPServerStats returns the servers the calls f selects went to, busiest
// first (an App's MCP usage).
func GetMCPServerStats(db *gorm.DB, f MCPStatsFilter) ([]MCPServerRow, error) {
	var rows []struct {
		MCPServerID  uint
		ServerName   *string
		ConnectionID uint
		Calls        int64
		Errors       int64
		AvgMs        float64
		LastSeen     ScanTime
	}
	if err := f.apply(db.Model(&models.MCPCallRecord{})).
		Joins("LEFT JOIN mcp_servers ON mcp_servers.id = mcp_call_records.mcp_server_id").
		Where("mcp_call_records.mcp_server_id IS NOT NULL").
		Select("mcp_call_records.mcp_server_id AS mcp_server_id, MAX(mcp_servers.name) AS server_name, " +
			"MAX(mcp_call_records.connection_id) AS connection_id, " + aggSelect).
		Group("mcp_call_records.mcp_server_id").
		Order("calls DESC, mcp_server_id").
		Limit(maxServerRows).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = fmt.Sprint(r.MCPServerID)
	}
	pct, err := mcpPercentiles(db, f, "mcp_server_id", keys)
	if err != nil {
		return nil, err
	}
	out := make([]MCPServerRow, len(rows))
	for i, r := range rows {
		name := ""
		if r.ServerName != nil {
			name = *r.ServerName
		}
		out[i] = MCPServerRow{MCPServerID: r.MCPServerID, ServerName: name, ConnectionID: r.ConnectionID, Calls: r.Calls, Errors: r.Errors,
			ErrorRate: rate(r.Errors, r.Calls), P95Ms: pct[keys[i]].p95, LastSeenAt: timePtr(r.LastSeen)}
	}
	return out, nil
}

// overviewTopTools is how many tools the overview lists.
const overviewTopTools = 10

// GetMCPOverview returns the summary of all MCP traffic in the range and its
// busiest tools.
func GetMCPOverview(db *gorm.DB, f MCPStatsFilter) (*MCPOverview, error) {
	sum, err := GetMCPSummary(db, f)
	if err != nil {
		return nil, err
	}
	out := &MCPOverview{MCPSummary: *sum, TopTools: []MCPTopTool{}}
	if sum.Calls == 0 {
		return out, nil
	}
	var rows []struct {
		MCPServerID   uint
		ServerName    *string
		PrimitiveType string
		PrimitiveName string
		Calls         int64
		Errors        int64
	}
	if err := f.apply(db.Model(&models.MCPCallRecord{})).
		Joins("LEFT JOIN mcp_servers ON mcp_servers.id = mcp_call_records.mcp_server_id").
		Where("mcp_call_records.mcp_server_id IS NOT NULL").
		Select("mcp_call_records.mcp_server_id AS mcp_server_id, MAX(mcp_servers.name) AS server_name, " +
			"COALESCE(primitive_type, '') AS primitive_type, COALESCE(primitive_name, '') AS primitive_name, " +
			"COUNT(*) AS calls, COALESCE(SUM(" + mcpErrorExpr + "), 0) AS errors").
		Group("mcp_call_records.mcp_server_id, primitive_type, primitive_name").
		Order("calls DESC, mcp_server_id, primitive_name").
		Limit(overviewTopTools).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		name := ""
		if r.ServerName != nil {
			name = *r.ServerName
		}
		out.TopTools = append(out.TopTools, MCPTopTool{MCPServerID: r.MCPServerID, ServerName: name, PrimitiveType: r.PrimitiveType,
			PrimitiveName: r.PrimitiveName, Calls: r.Calls, Errors: r.Errors, ErrorRate: rate(r.Errors, r.Calls)})
	}
	return out, nil
}

type percentilePair struct{ p50, p95 float64 }

// mcpPercentiles returns the interpolated p50 and p95 of latency_total_ms
// per group (keyExpr, rendered as text), for the calls f selects. An empty
// keyExpr is one group, keyed "". keys, when not nil, limits the groups
// computed (a page of rows).
func mcpPercentiles(db *gorm.DB, f MCPStatsFilter, keyExpr string, keys []string) (map[string]percentilePair, error) {
	out := map[string]percentilePair{}
	if keys != nil && len(keys) == 0 {
		return out, nil
	}
	// A constant groups everything (Postgres refuses GROUP BY on one, so
	// the single group is not grouped there).
	single := keyExpr == ""
	if single {
		keyExpr = "''"
	}
	scope := func(q *gorm.DB) *gorm.DB {
		q = f.apply(q)
		if keys != nil {
			q = q.Where(keyExpr+" IN ?", keys)
		}
		return q
	}
	if isPostgres(db) {
		var rows []struct {
			K   string
			P50 float64
			P95 float64
		}
		q := scope(db.Model(&models.MCPCallRecord{})).
			Select("CAST(" + keyExpr + " AS TEXT) AS k, " +
				"COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_total_ms), 0) AS p50, " +
				"COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_total_ms), 0) AS p95")
		if !single {
			q = q.Group(keyExpr)
		}
		if err := q.Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.K] = percentilePair{p50: r.P50, p95: r.P95}
		}
		return out, nil
	}

	// SQLite has no percentile_cont: rank each group's latencies with window
	// functions and pick the two values either side of the percentile's
	// position, (n-1)*p, interpolating between them in Go.
	ranked := scope(db.Model(&models.MCPCallRecord{})).
		Select("CAST(" + keyExpr + " AS TEXT) AS k, latency_total_ms AS v, " +
			"ROW_NUMBER() OVER (PARTITION BY " + keyExpr + " ORDER BY latency_total_ms) AS rn, " +
			"COUNT(*) OVER (PARTITION BY " + keyExpr + ") AS n")
	pick := func(p string) string {
		pos := "(" + p + " * (n - 1))"
		lo := "CAST(" + pos + " AS INTEGER) + 1"
		hi := "MIN(" + lo + " + 1, n)"
		return "MAX(CASE WHEN rn = " + lo + " THEN v END) AS lo" + strings.ReplaceAll(p, ".", "") + ", " +
			"MAX(CASE WHEN rn = " + hi + " THEN v END) AS hi" + strings.ReplaceAll(p, ".", "") + ", " +
			"MAX(" + pos + " - CAST(" + pos + " AS INTEGER)) AS fr" + strings.ReplaceAll(p, ".", "")
	}
	var rows []struct {
		K     string
		Lo05  float64
		Hi05  float64
		Fr05  float64
		Lo095 float64
		Hi095 float64
		Fr095 float64
	}
	if err := db.Table("(?) AS ranked", ranked).
		Select("k, " + pick("0.5") + ", " + pick("0.95")).
		Group("k").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.K] = percentilePair{
			p50: r.Lo05 + r.Fr05*(r.Hi05-r.Lo05),
			p95: r.Lo095 + r.Fr095*(r.Hi095-r.Lo095),
		}
	}
	return out, nil
}
