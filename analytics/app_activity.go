package analytics

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// AppActivity is the portal overview's per-app activity summary: when the
// app last completed an LLM interaction and how many it made in the window.
//
// It reads llm_chat_records, the same table the app page's token, cost and
// interaction charts and the budget spend are computed from, so the overview
// never disagrees with the app page (proxy_logs also records rejected and
// failed attempts, which the app page does not count). One grouped query
// over the caller's apps, so the overview costs one round trip however many
// apps there are.
type AppActivity struct {
	AppID        uint
	LastAccessAt *time.Time
	Requests     int64
}

type appActivityRow struct {
	AppID      uint
	LastAccess ScanTime
	Requests   int64
}

// GetAppActivity returns activity keyed by app id for the given apps. Apps
// with no chat records are absent from the map.
func GetAppActivity(db *gorm.DB, appIDs []uint, since time.Time) (map[uint]*AppActivity, error) {
	out := make(map[uint]*AppActivity)
	if len(appIDs) == 0 {
		return out, nil
	}
	var rows []appActivityRow
	err := db.Model(&models.LLMChatRecord{}).
		Select("app_id, MAX(time_stamp) AS last_access, "+
			"SUM(CASE WHEN time_stamp >= ? THEN 1 ELSE 0 END) AS requests", since).
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
