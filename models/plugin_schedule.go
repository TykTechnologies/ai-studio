package models

import (
	"time"
)

// PluginSchedule represents a cron-based scheduled task for a plugin
type PluginSchedule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  uint      `gorm:"index;not null" json:"plugin_id"`
	Plugin    Plugin    `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE" json:"plugin,omitempty"`

	ManifestScheduleID string `gorm:"index;not null" json:"schedule_id"` // From manifest (e.g., "sync-repos")
	Name               string `gorm:"not null" json:"name"`              // Human-readable name
	CronExpr           string `gorm:"not null" json:"cron_expr"`         // Cron expression
	Timezone           string `gorm:"default:'UTC'" json:"timezone"`     // Timezone for cron evaluation
	Enabled            bool   `gorm:"default:true" json:"enabled"`       // Whether schedule is enabled
	Config             string `gorm:"type:text" json:"config"`           // JSON config from manifest
	TimeoutSeconds     int    `gorm:"default:60" json:"timeout_seconds"` // Max execution time in seconds

	LastRun    *time.Time `json:"last_run,omitempty"`
	NextRun    *time.Time `json:"next_run,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PluginScheduleExecution represents a single execution of a scheduled task
type PluginScheduleExecution struct {
	ID                 uint           `gorm:"primaryKey" json:"id"`
	PluginScheduleID   uint           `gorm:"index;not null" json:"plugin_schedule_id"` // FK to plugin_schedules.id
	Schedule           PluginSchedule `gorm:"foreignKey:PluginScheduleID;constraint:OnDelete:CASCADE" json:"schedule,omitempty"`
	PluginID           uint           `gorm:"index;not null" json:"plugin_id"` // Track plugin for cleanup

	Status      string     `gorm:"index;not null;default:'pending'" json:"status"` // "pending", "running", "completed", "failed", "timeout"
	StartedAt   time.Time  `gorm:"index;not null" json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	LockedBy    string     `gorm:"index" json:"locked_by,omitempty"`               // Instance ID that owns this execution

	Success  bool   `json:"success"`
	Error    string `gorm:"type:text" json:"error,omitempty"`
	Duration int64  `json:"duration"` // Milliseconds

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SchedulerLease represents the leader election lease for scheduler service
type SchedulerLease struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	InstanceID  string    `gorm:"uniqueIndex;not null" json:"instance_id"` // hostname-pid
	LeaderID    string    `gorm:"index;not null" json:"leader_id"`         // Current leader instance
	ExpiresAt   time.Time `gorm:"index;not null" json:"expires_at"`        // Leader lease expiry
	HeartbeatAt time.Time `json:"heartbeat_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName returns table name for PluginSchedule
func (PluginSchedule) TableName() string {
	return "plugin_schedules"
}

// TableName returns table name for PluginScheduleExecution
func (PluginScheduleExecution) TableName() string {
	return "plugin_schedule_executions"
}

// TableName returns table name for SchedulerLease
func (SchedulerLease) TableName() string {
	return "scheduler_leases"
}
