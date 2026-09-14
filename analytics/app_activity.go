package analytics

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// AppActivity is the portal overview's per-app activity summary: when the
// app's credential last reached the gateway and how many requests it made in
// the window. Both come from proxy_logs in one grouped query over the caller's
// apps, so the overview costs one round trip however many apps there are.
type AppActivity struct {
	AppID        uint
	LastAccessAt *time.Time
	// Requests counts primary attempts only (failover_attempt = 0), the same
	// rule the analytics pages use, so a request that failed over is one
	// request and not one per rung.
	Requests int64
}

type appActivityRow struct {
	AppID      uint
	LastAccess ScanTime
	Requests   int64
}

// GetAppActivity returns activity keyed by app id for the given apps. Apps
// with no proxy log rows are absent from the map.
func GetAppActivity(db *gorm.DB, appIDs []uint, since time.Time) (map[uint]*AppActivity, error) {
	out := make(map[uint]*AppActivity)
	if len(appIDs) == 0 {
		return out, nil
	}
	var rows []appActivityRow
	err := db.Model(&models.ProxyLog{}).
		Select("app_id, MAX(time_stamp) AS last_access, "+
			"SUM(CASE WHEN time_stamp >= ? AND failover_attempt = 0 THEN 1 ELSE 0 END) AS requests", since).
		Where("app_id IN ?", appIDs).
		Group("app_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		activity := &AppActivity{AppID: row.AppID, Requests: row.Requests}
		if !row.LastAccess.IsZero() {
			last := row.LastAccess.Time
			activity.LastAccessAt = &last
		}
		out[row.AppID] = activity
	}
	return out, nil
}
