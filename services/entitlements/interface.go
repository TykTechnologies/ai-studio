// Package entitlements is the hook through which a license limits what a
// deployment may create: Apps, users with admin or portal access, and edge
// namespaces, plus the per-user App quota of self-service creation.
//
// Core only wraps its create and grant points in this interface and renders
// the opaque data it returns. Every rule (which limits exist, how they are
// counted, when a notice shows, what a refusal says) lives in the enterprise
// implementation, registered through the factory in this package.
//
// Community Edition ships a stub that allows everything: guards just run the
// mutation, every namespace is licensed and every grant may proceed. Its UI
// data calls answer ErrEnterpriseFeature, which handlers turn into 402.
package entitlements

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ErrEnterpriseFeature is returned by Community Edition for the report,
// notice and team quota calls.
var ErrEnterpriseFeature = errors.New("license limits and App quotas are an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// Team App quota errors. Handlers map ErrInvalidQuota onto 400 and
// ErrTeamNotFound onto 404.
var (
	ErrInvalidQuota = errors.New("invalid App quota: give a non-negative max_apps_per_member, or set unlimited")
	ErrTeamNotFound = errors.New("team not found")
	// ErrUnknownNotice refuses dismissing a key that is not a notice key
	// (handlers map it onto 400).
	ErrUnknownNotice = errors.New("unknown notice")
)

// Seat describes access an automatic provisioning path (SSO, host identity,
// self-registration) is about to give a user, so the implementation can say
// whether it fits before the grant is made. Set the fields being granted.
type Seat struct {
	// UserID is the user receiving the access; 0 for a user not yet created.
	UserID uint `json:"user_id"`
	// Email identifies the user in the refusal's log line and notice.
	Email string `json:"email"`
	// Portal grants portal access (User.ShowPortal).
	Portal bool `json:"portal"`
	// Admin grants the admin flag (User.IsAdmin).
	Admin bool `json:"admin"`
	// GroupID adds the user to this team, whose role bindings may give
	// admin access; 0 for none.
	GroupID uint `json:"group_id"`
}

// Limit is one licensed quantity and how much of it is in use.
type Limit struct {
	// Key identifies the limit (stable, for the UI's React keys and tests).
	Key   string `json:"key"`
	Label string `json:"label"`
	Used  int64  `json:"used"`
	// Max is the licensed maximum; ignore it when Unlimited.
	Max       int64 `json:"max"`
	Unlimited bool  `json:"unlimited"`
}

// Feature is one licensed feature switch.
type Feature struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
}

// NamespaceSlot is a namespace holding one of the licensed namespace slots.
type NamespaceSlot struct {
	Namespace string    `json:"namespace"`
	ClaimedAt time.Time `json:"claimed_at"`
	// ConnectedEdges counts the namespace's edges; a slot with connected
	// edges cannot be released.
	ConnectedEdges int64 `json:"connected_edges"`
}

// Report is everything the License page shows. Core renders it as given.
type Report struct {
	Valid     bool       `json:"valid"`
	SKUs      []string   `json:"skus"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	DaysLeft  int        `json:"days_left"`
	Features  []Feature  `json:"features"`
	Limits    []Limit    `json:"limits"`
	// Namespaces lists the namespaces holding a slot.
	Namespaces []NamespaceSlot `json:"namespaces"`
}

// Notice is a near-limit message for one user. Key identifies it for
// DismissNotice; a dismissed notice does not come back for the same key.
type Notice struct {
	Key       string `json:"key"`
	Message   string `json:"message"`
	Label     string `json:"label"`
	Used      int64  `json:"used"`
	Max       int64  `json:"max"`
	Unlimited bool   `json:"unlimited"`
}

// TeamAppQuota is a team's per-member App quota for self-service creation.
type TeamAppQuota struct {
	GroupID uint `json:"group_id"`
	// Set reports whether the team has its own quota; otherwise its members
	// get the global default.
	Set              bool `json:"set"`
	MaxAppsPerMember *int `json:"max_apps_per_member"`
	Unlimited        bool `json:"unlimited"`
	// The global default, for showing what members inherit.
	DefaultMax       int64 `json:"default_max"`
	DefaultUnlimited bool  `json:"default_unlimited"`
}

// TeamAppQuotaInput sets a team's per-member App quota: a number, or
// Unlimited.
type TeamAppQuotaInput struct {
	MaxAppsPerMember *int `json:"max_apps_per_member"`
	Unlimited        bool `json:"unlimited"`
}

// Service is the entitlements contract shared by both editions.
//
// The guards wrap a mutation: the implementation may lock, count before and
// after fn, and roll back with an *ExceededError when fn took a limit past
// its maximum. fn must make every write through the *gorm.DB it is given.
type Service interface {
	// GuardAppCreate wraps the creation of an App owned by owner.
	// selfService is true for portal and plugin SDK creates, which are
	// also subject to the per-user App quota; admin creates pass false.
	GuardAppCreate(tx *gorm.DB, owner uint, selfService bool, fn func(*gorm.DB) error) error
	// GuardUserChange wraps any change that may give users admin or portal
	// access: user create/update, team membership, role bindings.
	GuardUserChange(tx *gorm.DB, fn func(*gorm.DB) error) error

	// AdmitNamespace claims a namespace slot for ns if it has none and one
	// is free. It reports whether ns is licensed (the default namespace
	// always is).
	AdmitNamespace(tx *gorm.DB, ns string) (licensed bool, err error)
	// IsNamespaceLicensed reports whether ns holds a slot (or is the
	// default namespace). It never claims one.
	IsNamespaceLicensed(db *gorm.DB, ns string) bool
	// LicensedNamespaceScope narrows a query on edge_instances to edges in
	// licensed namespaces.
	LicensedNamespaceScope(q *gorm.DB) *gorm.DB
	// ReleaseNamespace frees ns's slot.
	ReleaseNamespace(db *gorm.DB, ns string) error

	// CanGrant reports whether an automatic provisioning path may give the
	// seat. When it may not, the caller leaves the access out rather than
	// failing the login; the implementation logs the refusal and tells the
	// Owners. The caller still makes its change under GuardUserChange.
	CanGrant(db *gorm.DB, seat Seat) bool

	// Report returns the License page data.
	Report(db *gorm.DB) (*Report, error)
	// Notices returns the near-limit notices u has not dismissed.
	Notices(db *gorm.DB, u *models.User) ([]Notice, error)
	// DismissNotice hides the notice with key for u for good.
	DismissNotice(db *gorm.DB, u *models.User, key string) error

	// GetTeamAppQuota returns a team's per-member App quota.
	GetTeamAppQuota(db *gorm.DB, groupID uint) (*TeamAppQuota, error)
	// SetTeamAppQuota creates or replaces a team's per-member App quota.
	SetTeamAppQuota(db *gorm.DB, groupID uint, q TeamAppQuotaInput) error
	// DeleteTeamAppQuota removes a team's own quota, so its members get the
	// global default again.
	DeleteTeamAppQuota(db *gorm.DB, groupID uint) error
	// UserAppQuota returns how many Apps a user owns and how many their
	// quota allows for self-service creation.
	UserAppQuota(db *gorm.DB, userID uint) (used int64, max int64, unlimited bool, err error)
}
