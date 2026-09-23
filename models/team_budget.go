package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Team budget enforcement modes. A team in alert-only mode is notified when
// its spend crosses a threshold; a hard-blocking team also has every one of
// its Apps refused once the ceiling is reached.
const (
	TeamBudgetAlertOnly = "alert_only"
	TeamBudgetHardBlock = "hard_block"
)

// TeamBudget holds the budget of one team (Group). Enterprise; the table
// exists in CE and is used there for nothing but reporting.
//
// MonthlyBudget semantics (as for App and LLM budgets, nil is "no limit" and
// 0 is zero):
//   - no row, or a nil budget: the team is unmanaged (no pool, no ceiling);
//   - 0: an empty pool and a zero ceiling; new Apps are allocated nothing;
//   - > 0: the pool Apps draw from, and the ceiling on the team's total spend.
//
// No gorm.Model: the row is deleted outright with its team, so a soft-delete
// tombstone can never block the unique group_id of a recreated row.
type TeamBudget struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	GroupID              uint       `json:"group_id" gorm:"not null;uniqueIndex"`
	MonthlyBudget        *float64   `json:"monthly_budget"`
	BudgetStartDate      *time.Time `json:"budget_start_date"`
	DefaultAppAllocation *float64   `json:"default_app_allocation"`
	Enforcement          string     `json:"enforcement" gorm:"size:16;not null;default:'alert_only'"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// IsManaged reports whether the team has a budget at all.
func (tb *TeamBudget) IsManaged() bool {
	return tb != nil && tb.MonthlyBudget != nil
}

// HardBlocks reports whether reaching the budget refuses the team's Apps.
func (tb *TeamBudget) HardBlocks() bool {
	return tb.IsManaged() && tb.Enforcement == TeamBudgetHardBlock
}

// GetTeamBudget returns the budget row of a team, or nil when it has none.
func GetTeamBudget(db *gorm.DB, groupID uint) (*TeamBudget, error) {
	var tb TeamBudget
	err := db.Where("group_id = ?", groupID).First(&tb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tb, nil
}

// TeamBudgetSettings is the single row holding the global team budget switch.
// While the switch is off, Apps and spend are still attributed to teams (for
// reporting) but nothing is allocated or enforced.
type TeamBudgetSettings struct {
	ID      uint `gorm:"primaryKey"`
	Enabled bool `gorm:"not null;default:false"`
	// AttributionBackfilled records that apps and spend recorded before
	// team attribution existed have been stamped with their team.
	AttributionBackfilled bool `gorm:"not null;default:false"`
	// ZeroBudgetsCleared records that App and LLM budgets of 0, which meant
	// "no limit" before 0 came to mean zero, were rewritten to nil.
	ZeroBudgetsCleared bool `gorm:"not null;default:false"`
	UpdatedAt          time.Time
}

const teamBudgetSettingsID = 1

// GetTeamBudgetSettings returns the settings row, or the zero value (switch
// off) when it has never been written.
func GetTeamBudgetSettings(db *gorm.DB) (*TeamBudgetSettings, error) {
	var s TeamBudgetSettings
	err := db.First(&s, teamBudgetSettingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &TeamBudgetSettings{ID: teamBudgetSettingsID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveTeamBudgetSettings upserts the settings row.
func SaveTeamBudgetSettings(db *gorm.DB, s *TeamBudgetSettings) error {
	s.ID = teamBudgetSettingsID
	// Save on a primary key inserts when the row is missing. Column updates
	// are explicit so a false switch is written, not skipped as a zero value.
	var existing int64
	if err := db.Model(&TeamBudgetSettings{}).Where("id = ?", s.ID).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		return db.Create(s).Error
	}
	return db.Model(&TeamBudgetSettings{}).Where("id = ?", s.ID).Updates(map[string]interface{}{
		"enabled":                s.Enabled,
		"attribution_backfilled": s.AttributionBackfilled,
		"zero_budgets_cleared":   s.ZeroBudgetsCleared,
		"updated_at":             time.Now(),
	}).Error
}

// ResolveBudgetTeam returns the team an App created by (or chat spend of)
// the given user is attributed to, or nil when no team applies:
//
//  1. the user's budget-holding team, while they are still a member of it
//     (SSO rewrites memberships at every login, so a stale choice is skipped);
//  2. otherwise the user's first non-Default team, by lowest team ID;
//  3. otherwise the Default team.
func ResolveBudgetTeam(db *gorm.DB, userID uint) (*uint, error) {
	if userID == 0 {
		return nil, nil
	}
	teams, err := ResolveBudgetTeams(db, []uint{userID})
	if err != nil {
		return nil, err
	}
	return teams[userID], nil
}

// resolveChunk bounds the IDs in one IN list (SQLite allows 999 variables).
const resolveChunk = 500

// usersByTeam inverts a user -> team map, dropping users with no team, so
// a backfill writes once per team rather than once per user.
func usersByTeam(teams map[uint]*uint) map[uint][]uint {
	out := map[uint][]uint{}
	for userID, teamID := range teams {
		if teamID != nil {
			out[*teamID] = append(out[*teamID], userID)
		}
	}
	return out
}

// chunkIDs splits IDs into IN-list sized chunks.
func chunkIDs(ids []uint) [][]uint {
	var out [][]uint
	for start := 0; start < len(ids); start += resolveChunk {
		end := start + resolveChunk
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[start:end])
	}
	return out
}

// ResolveBudgetTeams applies ResolveBudgetTeam to many users with a fixed
// number of queries: their budget teams, their memberships of live teams,
// and the Default team. Users that do not exist (or have no team at all and
// no Default team exists) map to nil.
func ResolveBudgetTeams(db *gorm.DB, userIDs []uint) (map[uint]*uint, error) {
	out := make(map[uint]*uint, len(userIDs))
	ids := make([]uint, 0, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		if _, seen := out[id]; !seen {
			out[id] = nil
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	type membership struct {
		UserID  uint
		GroupID uint
		Name    string
	}
	budgetTeam := map[uint]*uint{}
	exists := map[uint]bool{}
	member := map[uint]map[uint]bool{}
	firstOther := map[uint]uint{}
	for start := 0; start < len(ids); start += resolveChunk {
		end := start + resolveChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		var users []User
		if err := db.Select("id", "budget_team_id").Where("id IN ?", chunk).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, u := range users {
			exists[u.ID] = true
			budgetTeam[u.ID] = u.BudgetTeamID
		}

		var rows []membership
		if err := db.Table("user_groups").
			Select("user_groups.user_id AS user_id, groups.id AS group_id, groups.name AS name").
			Joins("JOIN groups ON groups.id = user_groups.group_id AND groups.deleted_at IS NULL").
			Where("user_groups.user_id IN ?", chunk).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			if member[r.UserID] == nil {
				member[r.UserID] = map[uint]bool{}
			}
			member[r.UserID][r.GroupID] = true
			if r.Name != DefaultGroupName {
				if cur, ok := firstOther[r.UserID]; !ok || r.GroupID < cur {
					firstOther[r.UserID] = r.GroupID
				}
			}
		}
	}

	var defaultID *uint
	needDefault := false
	for _, id := range ids {
		if !exists[id] {
			continue
		}
		// 1. The budget team, while the user is still a member of it (SSO
		//    rewrites memberships, so a stale choice is skipped).
		if bt := budgetTeam[id]; bt != nil && member[id][*bt] {
			v := *bt
			out[id] = &v
			continue
		}
		// 2. The first team other than Default, by lowest ID.
		if g, ok := firstOther[id]; ok {
			v := g
			out[id] = &v
			continue
		}
		needDefault = true
	}
	if needDefault {
		var def Group
		err := db.Select("id").Where("name = ?", DefaultGroupName).First(&def).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if err == nil {
			defaultID = &def.ID
		}
		// 3. The Default team.
		for _, id := range ids {
			if exists[id] && out[id] == nil && defaultID != nil {
				v := *defaultID
				out[id] = &v
			}
		}
	}
	return out, nil
}

// BackfillTeamAttribution stamps Apps and spend recorded before team
// attribution existed. It runs once: the settings row remembers it ran.
// Existing Apps keep their budgets and their (empty) budget source, so no
// traffic changes; only reporting gains the team.
func BackfillTeamAttribution(db *gorm.DB) error {
	settings, err := GetTeamBudgetSettings(db)
	if err != nil {
		return err
	}
	if settings.AttributionBackfilled {
		return nil
	}

	var owners []uint
	if err := db.Model(&App{}).Unscoped().Where("team_id IS NULL").Distinct().Pluck("user_id", &owners).Error; err != nil {
		return err
	}
	ownerTeams, err := ResolveBudgetTeams(db, owners)
	if err != nil {
		return err
	}
	for teamID, users := range usersByTeam(ownerTeams) {
		for _, chunk := range chunkIDs(users) {
			if err := db.Model(&App{}).Unscoped().Where("team_id IS NULL AND user_id IN ?", chunk).
				Update("team_id", teamID).Error; err != nil {
				return err
			}
		}
	}

	// Proxy and edge spend follows its App.
	if err := db.Exec(`UPDATE llm_chat_records SET team_id = (SELECT apps.team_id FROM apps WHERE apps.id = llm_chat_records.app_id)
		WHERE team_id IS NULL AND app_id <> 0`).Error; err != nil {
		return err
	}

	// Chat spend has no App; it follows the chatting user.
	var chatUsers []uint
	if err := db.Model(&LLMChatRecord{}).Where("team_id IS NULL AND app_id = 0 AND user_id <> 0").
		Distinct().Pluck("user_id", &chatUsers).Error; err != nil {
		return err
	}
	chatTeams, err := ResolveBudgetTeams(db, chatUsers)
	if err != nil {
		return err
	}
	for teamID, users := range usersByTeam(chatTeams) {
		for _, chunk := range chunkIDs(users) {
			if err := db.Model(&LLMChatRecord{}).Where("team_id IS NULL AND app_id = 0 AND user_id IN ?", chunk).
				Update("team_id", teamID).Error; err != nil {
				return err
			}
		}
	}

	settings.AttributionBackfilled = true
	return SaveTeamBudgetSettings(db, settings)
}

// ClearLegacyZeroBudgets rewrites App and LLM budgets of 0 (or less) to nil.
// Budgets used to treat anything at or below 0 as "no limit"; now nil is "no
// limit" and 0 is a budget of zero, so existing zeros must not start
// blocking traffic. It runs once: the settings row remembers it ran, because
// afterwards a 0 is a deliberate zero budget.
func ClearLegacyZeroBudgets(db *gorm.DB) error {
	settings, err := GetTeamBudgetSettings(db)
	if err != nil {
		return err
	}
	if settings.ZeroBudgetsCleared {
		return nil
	}
	if err := db.Model(&App{}).Unscoped().Where("monthly_budget <= 0").
		Update("monthly_budget", nil).Error; err != nil {
		return err
	}
	if err := db.Model(&LLM{}).Unscoped().Where("monthly_budget <= 0").
		Update("monthly_budget", nil).Error; err != nil {
		return err
	}
	settings.ZeroBudgetsCleared = true
	return SaveTeamBudgetSettings(db, settings)
}

// TeamCostRow is one team's spend over a reporting window.
type TeamCostRow struct {
	TeamID   uint    `json:"team_id"`
	TeamName string  `json:"team_name"`
	Cost     float64 `json:"cost"`
	Tokens   int64   `json:"tokens"`
	Requests int64   `json:"requests"`
}
