// Package team_budget defines team (Group) level budgets: a monthly ceiling
// on the combined spend of a team's Apps, and an allocation pool that new
// Apps in the team draw their App budget from.
//
// Community Edition ships a no-op implementation: nothing is allocated or
// enforced and every report answers ErrEnterpriseFeature. Enterprise Edition
// registers the real one via the factory in this package.
package team_budget

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ErrEnterpriseFeature is returned by Community Edition for every operation
// that needs the enterprise implementation.
var ErrEnterpriseFeature = errors.New("team budgets are an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// Errors shared by both editions. Handlers map ErrValidation and
// ErrAllocationExceedsPool onto 400, ErrNotFound onto 404.
var (
	ErrValidation            = errors.New("invalid team budget input")
	ErrNotFound              = errors.New("team not found")
	ErrAllocationExceedsPool = errors.New("allocation exceeds the team's unallocated budget")
	// ErrAllocationExhausted refuses a request of an App whose team
	// allocation is zero (the pool was empty when it was created).
	ErrAllocationExhausted = errors.New("team allocation exhausted")
	// ErrTeamBudgetExceeded refuses a request of an App whose hard-blocking
	// team has reached its monthly budget.
	ErrTeamBudgetExceeded = errors.New("team monthly budget exceeded")
)

// Event topics published on the local bus (and so available to webhooks).
const (
	// TopicThreshold fires once per period when a team's spend crosses 80%
	// and again at 100% of its budget.
	TopicThreshold = "budget.team.threshold"
	// TopicOverAllocated fires when a budget change leaves a team's App
	// allocations above its budget.
	TopicOverAllocated = "budget.team.over_allocated"
)

// BudgetInput sets a team's budget. A nil MonthlyBudget makes the team
// unmanaged; zero is a managed but empty pool.
type BudgetInput struct {
	MonthlyBudget        *float64   `json:"monthly_budget"`
	BudgetStartDate      *time.Time `json:"budget_start_date"`
	DefaultAppAllocation *float64   `json:"default_app_allocation"`
	Enforcement          string     `json:"enforcement"`
}

// AppAllocation is one App's share of its team's budget in a report.
type AppAllocation struct {
	AppID        uint     `json:"app_id"`
	Name         string   `json:"name"`
	OwnerID      uint     `json:"owner_id"`
	OwnerEmail   string   `json:"owner_email"`
	Allocation   *float64 `json:"allocation"`
	BudgetSource string   `json:"budget_source"`
	Spent        float64  `json:"spent"`
	// Deleted Apps keep their spend in the period they made it, but no
	// longer hold an allocation.
	Deleted bool `json:"deleted"`
	// Blocked is a team-allocated App with nothing allocated.
	Blocked bool `json:"blocked"`
	// Uncapped is a live App with no budget of its own; its spend counts
	// towards the team ceiling but it holds no allocation.
	Uncapped bool `json:"uncapped"`
}

// Report is a team's budget position in its current period.
type Report struct {
	TeamID               uint       `json:"team_id"`
	TeamName             string     `json:"team_name"`
	Enabled              bool       `json:"enabled"` // global switch
	Managed              bool       `json:"managed"`
	MonthlyBudget        *float64   `json:"monthly_budget"`
	DefaultAppAllocation *float64   `json:"default_app_allocation"`
	Enforcement          string     `json:"enforcement"`
	BudgetStartDate      *time.Time `json:"budget_start_date"`
	PeriodStart          time.Time  `json:"period_start"`
	PeriodEnd            time.Time  `json:"period_end"`
	// Spent is all spend attributed to the team this period: its Apps
	// (including deleted ones) and its members' chat.
	Spent       float64 `json:"spent"`
	ChatSpent   float64 `json:"chat_spent"`
	Usage       float64 `json:"usage"` // percent of MonthlyBudget
	Allocated   float64 `json:"allocated"`
	Unallocated float64 `json:"unallocated"` // negative when over-allocated
	// OverBudget: spend has reached the budget. OverAllocated: App
	// allocations add up to more than the budget.
	OverBudget    bool            `json:"over_budget"`
	OverAllocated bool            `json:"over_allocated"`
	Apps          []AppAllocation `json:"apps"`
}

// TeamCosts is every team's spend over a reporting window.
type TeamCosts struct {
	Start time.Time            `json:"start"`
	End   time.Time            `json:"end"`
	Teams []models.TeamCostRow `json:"teams"`
	// Unattributed is spend no team could be resolved for.
	Unattributed models.TeamCostRow `json:"unattributed"`
}

// Service is the team budget contract shared by both editions.
type Service interface {
	// Enabled reports the global team budget switch. CE: always false.
	Enabled() bool
	// SetEnabled flips the global switch. Switching on gives the Default
	// team an empty managed pool (budget 0) unless it already has a budget.
	SetEnabled(enabled bool) error

	// GetTeamBudget returns a team's budget row, nil when it has none.
	GetTeamBudget(teamID uint) (*models.TeamBudget, error)
	// SetTeamBudget creates or replaces a team's budget.
	SetTeamBudget(teamID uint, in BudgetInput) (*models.TeamBudget, error)
	// DeleteTeamBudget removes a team's budget row (team deletion).
	DeleteTeamBudget(teamID uint) error
	// ResetTeamBudget starts a new budget period now.
	ResetTeamBudget(teamID uint) error

	// AllocateForNewApp draws the budget of an App about to be created from
	// its team's pool: a requested budget (app.MonthlyBudget) must fit the
	// unallocated pool, otherwise the team's default allocation is capped
	// at what is left. Either way the App becomes team-allocated.
	// app.TeamID must already be set. It is a no-op while the switch is
	// off or the team is unmanaged.
	AllocateForNewApp(app *models.App) error
	// ValidateAllocation checks an App budget (and optionally a move to
	// another team) against the destination team's unallocated pool. It
	// returns ErrAllocationExceedsPool when it does not fit.
	ValidateAllocation(app *models.App, newBudget *float64, newTeamID *uint) error
	// ReleaseAllocation returns an App's allocation to its pool while the
	// App itself lives on (orphaned Apps). Deleted Apps release theirs by
	// dropping out of the pool on soft delete.
	ReleaseAllocation(app *models.App) error
	// AdoptApp converts a manually budgeted App of a managed team into a
	// team allocation, drawing its current budget (or the team default)
	// from the pool.
	AdoptApp(appID uint) (*models.App, error)

	// CheckApp is the request-time check. It returns ErrAllocationExhausted
	// or ErrTeamBudgetExceeded when the App must be refused.
	CheckApp(app *models.App) error
	// AnalyzeTeamUsage checks the App's team against its alert thresholds
	// after spend was recorded. It never blocks the caller.
	AnalyzeTeamUsage(app *models.App)
	// EdgeBlocks lists the Apps edge gateways must refuse because of their
	// team, with the reason, for the budget sync to push down. Empty while
	// the switch is off.
	EdgeBlocks() (map[uint]string, error)

	// GetReport returns a team's position in its current period.
	GetReport(teamID uint) (*Report, error)
	// GetTeamCosts returns spend per team over the window.
	GetTeamCosts(start, end time.Time) (*TeamCosts, error)

	// ClearCache drops cached settings, budgets and spend.
	ClearCache()
}
