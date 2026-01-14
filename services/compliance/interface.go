package compliance

import (
	"time"
)

// ComplianceSummary contains high-level compliance metrics
type ComplianceSummary struct {
	AuthFailures      int     `json:"auth_failures"`
	AuthFailuresTrend float64 `json:"auth_failures_trend"` // % change from previous period
	PolicyViolations  int     `json:"policy_violations"`
	PolicyTrend       float64 `json:"policy_trend"`
	BudgetAlerts      int     `json:"budget_alerts"` // Apps >80% budget
	BudgetTrend       float64 `json:"budget_trend"`
	ErrorRate         float64 `json:"error_rate"` // % of 5xx errors
	ErrorTrend        float64 `json:"error_trend"`
	TotalRequests     int64   `json:"total_requests"`
}

// HighRiskApp represents an app that needs compliance attention
type HighRiskApp struct {
	AppID      uint     `json:"app_id"`
	AppName    string   `json:"app_name"`
	OwnerID    uint     `json:"owner_id"`
	OwnerEmail string   `json:"owner_email"`
	RiskScore  int      `json:"risk_score"`
	RiskLevel  string   `json:"risk_level"` // HIGH, MEDIUM, LOW
	Issues     []string `json:"issues"`     // Human-readable issue summaries
	// Breakdown of risk factors
	AuthFailures     int     `json:"auth_failures"`
	PolicyViolations int     `json:"policy_violations"`
	BudgetPercent    float64 `json:"budget_percent"`
	ErrorCount       int     `json:"error_count"`
}

// AccessIssue represents an access control violation
type AccessIssue struct {
	AppID        uint      `json:"app_id"`
	AppName      string    `json:"app_name"`
	ResponseCode int       `json:"response_code"`
	Count        int       `json:"count"`
	LastOccurred time.Time `json:"last_occurred"`
}

// AccessIssuesData contains aggregated access issue data
type AccessIssuesData struct {
	ByCode    map[int]int    `json:"by_code"`    // Count per response code
	ByApp     []AccessIssue  `json:"by_app"`     // Issues grouped by app
	Timeline  []TimelineData `json:"timeline"`   // Daily breakdown
	Total401  int            `json:"total_401"`
	Total403  int            `json:"total_403"`
	Total4xx  int            `json:"total_4xx"`
}

// TimelineData represents data for a specific date
type TimelineData struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// PolicyViolation represents policy/filter violations for an app
type PolicyViolation struct {
	AppID          uint      `json:"app_id"`
	AppName        string    `json:"app_name"`
	ViolationTypes []string  `json:"violation_types"` // filter_block, model_access, budget_exceeded, auth_failure
	Count          int       `json:"count"`
	LastOccurred   time.Time `json:"last_occurred"`
}

// PolicyViolationsData contains aggregated policy violation data
type PolicyViolationsData struct {
	FilterBlocks    []PolicyViolation `json:"filter_blocks"`
	ModelViolations []PolicyViolation `json:"model_violations"`
	Timeline        []TimelineData    `json:"timeline"`
	TotalBlocks     int               `json:"total_blocks"`
}

// BudgetAlert represents an app/LLM approaching or exceeding budget
type BudgetAlert struct {
	EntityID     uint    `json:"entity_id"`
	EntityType   string  `json:"entity_type"` // App or LLM
	Name         string  `json:"name"`
	OwnerID      uint    `json:"owner_id,omitempty"`
	OwnerEmail   string  `json:"owner_email,omitempty"`
	Budget       float64 `json:"budget"`
	Spent        float64 `json:"spent"`
	Percentage   float64 `json:"percentage"`
	Velocity     float64 `json:"velocity"` // Spending rate per day
	AlertLevel   string  `json:"alert_level"` // warning (>80%), critical (>95%)
}

// BudgetAlertsData contains budget compliance information
type BudgetAlertsData struct {
	Alerts        []BudgetAlert  `json:"alerts"`
	Timeline      []TimelineData `json:"timeline"` // Daily spending
	WarningCount  int            `json:"warning_count"`  // >80%
	CriticalCount int            `json:"critical_count"` // >95%
}

// ErrorData represents error metrics
type ErrorData struct {
	ByVendor  map[string]int `json:"by_vendor"`
	Timeline  []TimelineData `json:"timeline"`
	Total5xx  int            `json:"total_5xx"`
	ErrorRate float64        `json:"error_rate"`
}

// AppRiskProfile contains detailed compliance profile for a single app
type AppRiskProfile struct {
	AppID            uint              `json:"app_id"`
	AppName          string            `json:"app_name"`
	OwnerID          uint              `json:"owner_id"`
	OwnerEmail       string            `json:"owner_email"`
	RiskScore        int               `json:"risk_score"`
	RiskLevel        string            `json:"risk_level"`
	Summary          ComplianceSummary `json:"summary"`
	RecentViolations []ViolationEvent  `json:"recent_violations"`
	BudgetStatus     *BudgetAlert      `json:"budget_status,omitempty"`
}

// ViolationEvent represents a single compliance event
type ViolationEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	Type         string    `json:"type"` // auth_failure, policy_violation, budget_exceeded, error
	ResponseCode int       `json:"response_code,omitempty"`
	Details      string    `json:"details"`
}

// ViolationRecord represents an individual policy violation with full details
type ViolationRecord struct {
	ID            uint      `json:"id"`
	AppID         uint      `json:"app_id"`
	AppName       string    `json:"app_name"`
	Timestamp     time.Time `json:"timestamp"`
	ResponseCode  int       `json:"response_code"`
	ViolationType string    `json:"violation_type"` // filter_block, model_access, budget_exceeded, auth_failure
	FilterName    string    `json:"filter_name,omitempty"`
	ErrorDetail   string    `json:"error_detail"`
	Vendor        string    `json:"vendor"`
}

// Service defines the interface for compliance management.
// Community Edition provides stub implementations that return enterprise feature errors.
// Enterprise Edition provides full compliance monitoring and risk analysis.
type Service interface {
	// GetSummary returns high-level compliance metrics for the dashboard.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns aggregated compliance data
	GetSummary(startDate, endDate time.Time) (*ComplianceSummary, error)

	// GetHighRiskApps returns apps ranked by compliance risk.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns apps with risk scoring
	GetHighRiskApps(startDate, endDate time.Time, limit int) ([]HighRiskApp, error)

	// GetAccessIssues returns authentication and authorization failures.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns 401/403 breakdown by app and timeline
	GetAccessIssues(startDate, endDate time.Time, appID *uint) (*AccessIssuesData, error)

	// GetPolicyViolations returns filter blocks and model access violations.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns policy violation data
	GetPolicyViolations(startDate, endDate time.Time, appID *uint) (*PolicyViolationsData, error)

	// GetBudgetAlerts returns apps/LLMs approaching or exceeding budget limits.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns budget compliance data
	GetBudgetAlerts(startDate, endDate time.Time) (*BudgetAlertsData, error)

	// GetErrors returns error metrics by vendor.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns 5xx error data
	GetErrors(startDate, endDate time.Time, vendor *string) (*ErrorData, error)

	// GetAppRiskProfile returns detailed compliance profile for a single app.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns detailed app risk data
	GetAppRiskProfile(appID uint, startDate, endDate time.Time) (*AppRiskProfile, error)

	// ExportData exports compliance data in CSV format.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns CSV data for the specified view
	ExportData(startDate, endDate time.Time, view string) ([]byte, error)

	// GetViolationRecords returns individual violation records for drill-down view.
	// CE: Returns ErrEnterpriseFeature
	// ENT: Returns individual violation records with parsed details
	GetViolationRecords(startDate, endDate time.Time, appID *uint, limit int) ([]ViolationRecord, error)
}
