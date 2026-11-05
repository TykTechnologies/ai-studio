package models

import "time"

// The tables below hold the state of license limits and App quotas
// (services/entitlements). They are schema only: the enterprise
// implementation reads and writes them; Community Edition leaves them empty.

// LicensedNamespace is a non-default edge namespace holding one of the
// license's namespace slots. Rows are deleted outright when a slot is
// released, so the unique namespace can be claimed again.
type LicensedNamespace struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Namespace string    `json:"namespace" gorm:"size:255;not null;uniqueIndex"`
	ClaimedAt time.Time `json:"claimed_at" gorm:"not null"`
	// IdleSince is when the namespace last had a connected edge, while it
	// has none; nil while one is connected.
	IdleSince *time.Time `json:"idle_since"`
}

// NamespaceWait is a non-default namespace waiting for a licensed slot,
// since when. Rows are deleted outright when the namespace is admitted.
type NamespaceWait struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Namespace string    `json:"namespace" gorm:"size:255;not null;uniqueIndex"`
	Since     time.Time `json:"since" gorm:"not null"`
}

// TeamAppQuota is a team's (Group's) per-member quota of self-service Apps.
// Members of teams without a row get the global default. No gorm.Model: the
// row is deleted outright with its team (see TeamBudget).
type TeamAppQuota struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	GroupID          uint      `json:"group_id" gorm:"not null;uniqueIndex"`
	MaxAppsPerMember *int      `json:"max_apps_per_member"`
	Unlimited        bool      `json:"unlimited" gorm:"not null;default:false"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName fixes the plural (gorm's inflection leaves "quota" singular).
func (TeamAppQuota) TableName() string { return "team_app_quotas" }

// QuotaLock is a row to lock (SELECT ... FOR UPDATE) while counting and
// changing what a limit covers, so concurrent changes on any replica are
// serialised per limit.
type QuotaLock struct {
	Name string `gorm:"primaryKey;size:64"`
}

// UserNoticeDismissal records that a user dismissed a notice for good.
type UserNoticeDismissal struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"user_id" gorm:"not null;uniqueIndex:idx_user_notice_dismissals_user_key"`
	Key         string    `json:"key" gorm:"size:255;not null;uniqueIndex:idx_user_notice_dismissals_user_key"`
	DismissedAt time.Time `json:"dismissed_at" gorm:"not null"`
}
