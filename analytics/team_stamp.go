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
	if t == nil || rec == nil || rec.TeamID != nil {
		return
	}
	switch {
	case rec.AppID != 0:
		rec.TeamID = t.lookup(t.apps, rec.AppID, t.appTeam)
	case rec.UserID != 0:
		rec.TeamID = t.lookup(t.users, rec.UserID, func(id uint) (*uint, error) {
			return models.ResolveBudgetTeam(t.db, id)
		})
	}
}

func (t *teamStamper) lookup(cache map[uint]stampEntry, id uint, resolve func(uint) (*uint, error)) *uint {
	t.mu.Lock()
	if e, ok := cache[id]; ok && t.now().Sub(e.at) < teamStampTTL {
		t.mu.Unlock()
		return e.teamID
	}
	t.mu.Unlock()

	teamID, err := resolve(id)
	if err != nil {
		logger.Debugf("analytics: resolving team for id %d: %v", id, err)
		return nil
	}
	t.mu.Lock()
	cache[id] = stampEntry{teamID: teamID, at: t.now()}
	t.mu.Unlock()
	return teamID
}

func (t *teamStamper) appTeam(appID uint) (*uint, error) {
	var app models.App
	err := t.db.Unscoped().Select("id", "team_id").First(&app, appID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return app.TeamID, err
}
