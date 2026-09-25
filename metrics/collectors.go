package metrics

import (
	"database/sql"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// RegisterDBStats exports a connection pool's statistics as go_sql_* metrics
// labelled db_name=name. wait_count and wait_duration_seconds_total show
// callers queueing for a free connection.
func RegisterDBStats(name string, db *sql.DB) error {
	return Register(collectors.NewDBStatsCollector(db, name))
}

// RegisterGaugeFunc exports fn as a gauge read at scrape time.
func RegisterGaugeFunc(metricName, help string, fn func() float64) error {
	return Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: metricName, Help: help}, fn))
}

// RegisterCounterFunc exports fn, which must only ever increase, as a counter
// read at scrape time.
func RegisterCounterFunc(metricName, help string, fn func() float64) error {
	return Register(prometheus.NewCounterFunc(prometheus.CounterOpts{Name: metricName, Help: help}, fn))
}

// RegisterFileSize exports the size of the file at path as a gauge read at
// scrape time; a missing file reads as 0.
func RegisterFileSize(metricName, help, path string) error {
	return Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: metricName,
		Help: help,
	}, func() float64 {
		fi, err := os.Stat(path)
		if err != nil {
			return 0
		}
		return float64(fi.Size())
	}))
}
