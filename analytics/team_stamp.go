package analytics

import (
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// teamStampTTL bounds how long a record keeps being attributed to an App's
// (or user's) previous team after it moves.
const teamStampTTL = time.Minute

// teamStamper attributes spend records to a team before they are written:
// proxy and edge records follow their App's team, chat records (no App)
// follow the chatting user's budget team. It runs on the analytics writer
// goroutine, off the request path, and caches lookups briefly.
type teamStamper struct {
	db    *gorm.DB
	mu    sync.Mutex
	apps  map[uint]stampEntry
	users map[uint]stampEntry
	now   func() time.Time
	// swept is when expired entries were last dropped, so the caches hold
	// roughly the IDs seen in the last two TTLs rather than every ID ever.
	swept time.Time
}

type stampEntry struct {
	teamID *uint
	at     time.Time
}

func newTeamStamper(db *gorm.DB) *teamStamper {
	return &teamStamper{
		db:    db,
		apps:  map[uint]stampEntry{},
		users: map[uint]stampEntry{},
		now:   time.Now,
	}
}

// stamp sets rec.TeamID when the caller has not.
func (t *teamStamper) stamp(rec *models.LLMChatRecord) {
	t.stampBatch([]*models.LLMChatRecord{rec})
}

// stampBatch sets TeamID on every record the caller has not stamped. Cache
// misses are resolved together, so a batch costs a fixed handful of queries
// however many Apps and users it carries.
func (t *teamStamper) stampBatch(records []*models.LLMChatRecord) {
	if t == nil || len(records) == 0 {
		return
	}
	now := t.now()

	// What the cache cannot answer.
	var missingApps, missingUsers []uint
	t.mu.Lock()
	seenApp, seenUser := map[uint]bool{}, map[uint]bool{}
	for _, rec := range records {
		if rec == nil || rec.TeamID != nil {
			continue
		}
		switch {
		case rec.AppID != 0:
			if e, ok := t.apps[rec.AppID]; (!ok || now.Sub(e.at) >= teamStampTTL) && !seenApp[rec.AppID] {
				seenApp[rec.AppID] = true
				missingApps = append(missingApps, rec.AppID)
			}
		case rec.UserID != 0:
			if e, ok := t.users[rec.UserID]; (!ok || now.Sub(e.at) >= teamStampTTL) && !seenUser[rec.UserID] {
				seenUser[rec.UserID] = true
				missingUsers = append(missingUsers, rec.UserID)
			}
		}
	}
	t.mu.Unlock()

	appTeams, appErr := t.appTeams(missingApps)
	if appErr != nil {
		logger.Debugf("analytics: resolving teams of %d apps: %v", len(missingApps), appErr)
	}
	userTeams := map[uint]*uint{}
	if len(missingUsers) > 0 {
		var err error
		if userTeams, err = models.ResolveBudgetTeams(t.db, missingUsers); err != nil {
			logger.Debugf("analytics: resolving teams of %d users: %v", len(missingUsers), err)
			userTeams = nil
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked(now)
	// Cache every answer, including "no team", so unknown IDs are not
	// looked up again on every batch. A failed lookup is not cached.
	if appErr == nil {
		for _, id := range missingApps {
			t.apps[id] = stampEntry{teamID: appTeams[id], at: now}
		}
	}
	if userTeams != nil {
		for _, id := range missingUsers {
			t.users[id] = stampEntry{teamID: userTeams[id], at: now}
		}
	}
	for _, rec := range records {
		if rec == nil || rec.TeamID != nil {
			continue
		}
		switch {
		case rec.AppID != 0:
			rec.TeamID = copyTeamID(t.apps[rec.AppID].teamID)
		case rec.UserID != 0:
			rec.TeamID = copyTeamID(t.users[rec.UserID].teamID)
		}
	}
}

// sweepLocked drops expired entries at most once per TTL. t.mu is held.
func (t *teamStamper) sweepLocked(now time.Time) {
	if now.Sub(t.swept) < teamStampTTL {
		return
	}
	for id, e := range t.apps {
		if now.Sub(e.at) >= teamStampTTL {
			delete(t.apps, id)
		}
	}
	for id, e := range t.users {
		if now.Sub(e.at) >= teamStampTTL {
			delete(t.users, id)
		}
	}
	t.swept = now
}

// appTeams reads the team of each App, deleted ones included (their late
// records still belong to the team), in bounded IN lists.
func (t *teamStamper) appTeams(ids []uint) (map[uint]*uint, error) {
	out := make(map[uint]*uint, len(ids))
	for start := 0; start < len(ids); start += stampChunk {
		end := start + stampChunk
		if end > len(ids) {
			end = len(ids)
		}
		var apps []models.App
		if err := t.db.Unscoped().Select("id", "team_id").Where("id IN ?", ids[start:end]).Find(&apps).Error; err != nil {
			return nil, err
		}
		for _, a := range apps {
			out[a.ID] = a.TeamID
		}
	}
	return out, nil
}

// stampChunk bounds the IDs in one IN list (SQLite allows 999 variables).
const stampChunk = 500

// copyTeamID gives each record its own pointer, so no two records (or the
// cache) share one.
func copyTeamID(id *uint) *uint {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}
