package api

import (
	"context"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// handleHealth reports liveness and the process's FIPS 140-3 status
// ("fips": {enabled, version, enforced}). It never looks at the database:
// a replica whose database is down is not restarted for it.
func (a *API) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "fips": fipsinfo.Current()})
}

// SetDatabaseReadyGrace sets how long the database may be unreachable (or,
// for /ready?require=write, not writable) before /ready fails
// (DATABASE_READY_GRACE; set by pkg/studio). 0, the default, fails it as
// soon as the database does not answer, as before 2.3.
func (a *API) SetDatabaseReadyGrace(d time.Duration) {
	a.dbReadyGrace = d
}

// handleReadiness answers /ready and /readyz (see Readiness).
func (a *API) handleReadiness(c *gin.Context) {
	var db *gorm.DB
	if a.config != nil {
		db = a.config.DB
	}
	Readiness(db, a.dbReadyGrace)(c)
}

// Readiness is the readiness probe of a replica on db, with grace as
// DATABASE_READY_GRACE: the handler behind /ready and /readyz.
//
// With PostgreSQL the database's state comes from its dbstate.Tracker,
// which probes the server every few seconds, and the body says what it is
// ("database": writable | read_only | unreachable, "database_since"). A
// read-only database (a standby, or a primary being demoted) is ready: reads
// work, and a write fails in its own request. An unreachable one stays ready
// until grace has passed since the database stopped taking writes, so a
// failover shorter than that keeps every replica in the load balancer;
// after it, 503. ?require=write is ready only while the database is
// writable, with the same grace, for load balancers that should route only
// to a replica that can save.
//
// With a grace of 0 the database is pinged in the request, as before 2.3,
// and that ping decides; require=write also needs the tracker to see a
// writable database. With SQLite (no tracker) the answer is the ping alone,
// with the body of before 2.3.
func Readiness(db *gorm.DB, grace time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}
		tracker := dbstate.For(db)
		if tracker == nil {
			if !pingDatabase(c.Request.Context(), db) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
			return
		}

		requireWrite := c.Query("require") == "write"
		st := tracker.Status()
		var ready bool
		if grace <= 0 {
			ready = pingDatabase(c.Request.Context(), db)
			if !ready && st.State != dbstate.Unreachable {
				// The tracker has not probed since: the ping is newer.
				st = dbstate.Status{State: dbstate.Unreachable}
			}
			if requireWrite && st.State != dbstate.Writable {
				ready = false
			}
		} else {
			ready = databaseReady(st, grace, requireWrite, time.Now())
		}

		body := gin.H{"status": "ready", "database": string(st.State)}
		if !st.Since.IsZero() {
			body["database_since"] = st.Since.UTC().Format(time.RFC3339)
		}
		if !ready {
			body["status"] = "not ready"
			c.JSON(http.StatusServiceUnavailable, body)
			return
		}
		c.JSON(http.StatusOK, body)
	}
}

// databaseReady applies the readiness rule to the tracker's status: a
// writable database is ready; a read-only one is ready unless a write is
// required; otherwise it is ready only until grace has passed since the
// database stopped taking writes.
func databaseReady(st dbstate.Status, grace time.Duration, requireWrite bool, now time.Time) bool {
	switch {
	case st.State == dbstate.Writable:
		return true
	case st.State == dbstate.ReadOnly && !requireWrite:
		return true
	}
	return grace > 0 && !st.NotWritableSince.IsZero() && now.Sub(st.NotWritableSince) < grace
}

// pingDatabase is the readiness check before 2.3: the database answers.
func pingDatabase(ctx context.Context, db *gorm.DB) bool {
	sqlDB, err := db.DB()
	if err != nil {
		return false
	}
	return sqlDB.PingContext(ctx) == nil
}
