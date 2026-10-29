package analytics

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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

// AppSpendWindow names an app and the start of its budget period.
type AppSpendWindow struct {
	AppID uint
	Start time.Time
}

// GetAppSpending returns each app's spend between its own window start and
// `end`, by currency (budget.CurrencySQL), in the same units as the budget
// service (llm_chat_records.cost is stored x10000; see
// proxy/analyze_utils.go). A budget's currency is implied by its spend: read
// each app's figures with budget.SplitCurrency. One query for every app: each
// app contributes an (app_id, start) predicate, so a user with twenty apps
// costs one round trip rather than twenty. Apps with no records are absent.
func GetAppSpending(db *gorm.DB, windows []AppSpendWindow, end time.Time) (map[uint]map[string]float64, error) {
	out := make(map[uint]map[string]float64, len(windows))
	if len(windows) == 0 {
		return out, nil
	}
	query := db.Model(&models.LLMChatRecord{}).
		Select("app_id, "+budget.CurrencySQL+" AS currency, COALESCE(SUM(cost), 0) AS spent").
		Where("time_stamp <= ?", end)
	scope := db
	for i, w := range windows {
		if i == 0 {
			scope = scope.Where("(app_id = ? AND time_stamp >= ?)", w.AppID, w.Start)
		} else {
			scope = scope.Or("(app_id = ? AND time_stamp >= ?)", w.AppID, w.Start)
		}
	}
	var rows []struct {
		AppID    uint
		Currency string
		Spent    float64
	}
	if err := query.Where(scope).Group("app_id, " + budget.CurrencySQL).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AppID] = budget.AddCurrencies(out[row.AppID], map[string]float64{row.Currency: row.Spent / 10000.0})
	}
	return out, nil
}
