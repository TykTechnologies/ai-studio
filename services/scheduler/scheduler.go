package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PluginClient interface for executing scheduled tasks via gRPC
type PluginClient interface {
	ExecuteScheduledTask(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error)
}

// SchedulerService manages scheduled task execution for plugins
type SchedulerService struct {
	db                *gorm.DB
	pluginClient      PluginClient
	leaderElection    *LeaderElectionManager

	mu                sync.RWMutex
	schedules         map[uint]*ScheduleRunner // plugin_schedule_id -> runner

	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

// ScheduleRunner tracks a single scheduled task
type ScheduleRunner struct {
	schedule    *models.PluginSchedule
	cronEngine  *cron.Cron
	cronEntryID cron.EntryID
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(db *gorm.DB, pluginClient PluginClient) *SchedulerService {
	ctx, cancel := context.WithCancel(context.Background())

	return &SchedulerService{
		db:             db,
		pluginClient:   pluginClient,
		leaderElection: NewLeaderElectionManager(db),
		schedules:      make(map[uint]*ScheduleRunner),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the scheduler service (leader election loop)
func (s *SchedulerService) Start() error {
	log.Info().Msg("Starting scheduler service...")

	// Start leader election loop
	s.wg.Add(1)
	go s.leaderElectionLoop()

	return nil
}

// Stop gracefully shuts down the scheduler
func (s *SchedulerService) Stop() error {
	log.Info().Msg("Stopping scheduler service...")

	// Cancel context
	s.cancel()

	// Stop all cron engines
	s.mu.Lock()
	for _, runner := range s.schedules {
		if runner.cronEngine != nil {
			cronCtx := runner.cronEngine.Stop()
			<-cronCtx.Done()
		}
	}
	s.mu.Unlock()

	// Release leader lease
	if err := s.leaderElection.ReleaseLease(); err != nil {
		log.Warn().Err(err).Msg("Error releasing leader lease")
	}

	// Wait for goroutines
	s.wg.Wait()

	log.Info().Msg("Scheduler service stopped")
	return nil
}

// leaderElectionLoop runs leader election and scheduler loop
func (s *SchedulerService) leaderElectionLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Try to become leader immediately
	s.checkLeadershipAndRun()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkLeadershipAndRun()
		}
	}
}

// checkLeadershipAndRun checks if we're the leader and runs scheduler if so
func (s *SchedulerService) checkLeadershipAndRun() {
	isLeader, err := s.leaderElection.TryBecomeLeader()
	if err != nil {
		log.Error().Err(err).Msg("Leader election error")
		return
	}

	if isLeader {
		// We're the leader - load and register schedules
		log.Info().
			Str("instance_id", s.leaderElection.GetInstanceID()).
			Msg("This instance is the scheduler leader")
		s.loadAndRegisterSchedules()
	} else {
		// We're a follower - stop any running schedules
		s.mu.Lock()
		if len(s.schedules) > 0 {
			log.Info().
				Int("schedule_count", len(s.schedules)).
				Msg("This instance is a follower, stopping schedules")
			for _, runner := range s.schedules {
				if runner.cronEngine != nil {
					runner.cronEngine.Stop()
				}
			}
			s.schedules = make(map[uint]*ScheduleRunner)
		}
		s.mu.Unlock()
	}
}

// loadAndRegisterSchedules loads schedules from database and registers with cron
func (s *SchedulerService) loadAndRegisterSchedules() {
	// Load all enabled schedules from database
	var schedules []models.PluginSchedule
	if err := s.db.Where("enabled = ?", true).Preload("Plugin").Find(&schedules).Error; err != nil {
		log.Error().Err(err).Msg("Failed to load schedules from database")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Build map of schedule IDs that should exist
	validScheduleIDs := make(map[uint]bool)
	for i := range schedules {
		validScheduleIDs[schedules[i].ID] = true
	}

	// Unregister schedules that no longer exist in database (deleted plugins)
	for scheduleID, runner := range s.schedules {
		if !validScheduleIDs[scheduleID] {
			// Schedule was deleted - stop cron engine
			if runner.cronEngine != nil {
				runner.cronEngine.Stop()
			}
			delete(s.schedules, scheduleID)
			log.Info().
				Uint("schedule_id", scheduleID).
				Msg("Unregistered deleted schedule from cron engine")
		}
	}

	// Register new schedules
	for i := range schedules {
		schedule := &schedules[i]

		// Skip if already registered
		if _, exists := s.schedules[schedule.ID]; exists {
			continue
		}

		// Register schedule
		if err := s.registerScheduleUnsafe(schedule); err != nil {
			log.Error().
				Err(err).
				Uint("schedule_id", schedule.ID).
				Str("schedule_name", schedule.Name).
				Msg("Failed to register schedule")
		}
	}
}

// registerScheduleUnsafe registers a schedule with cron (must be called with lock held)
func (s *SchedulerService) registerScheduleUnsafe(schedule *models.PluginSchedule) error {
	// Load timezone
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		log.Warn().
			Err(err).
			Str("timezone", schedule.Timezone).
			Uint("schedule_id", schedule.ID).
			Msg("Invalid timezone, using UTC fallback")
		loc = time.UTC
	}

	// Create cron engine with timezone support
	// Note: Using standard 5-field cron format (minute hour day month weekday)
	// For second precision, use 6-field format in manifest
	cronEngine := cron.New(
		cron.WithLocation(loc),
	)

	// Parse and register cron expression
	entryID, err := cronEngine.AddFunc(schedule.CronExpr, func() {
		s.executeSchedule(schedule)
	})

	if err != nil {
		return fmt.Errorf("invalid cron expression '%s': %w", schedule.CronExpr, err)
	}

	// Start this schedule's cron engine
	cronEngine.Start()

	// Track runner
	s.schedules[schedule.ID] = &ScheduleRunner{
		schedule:    schedule,
		cronEngine:  cronEngine,
		cronEntryID: entryID,
	}

	// Update next run time
	nextEntry := cronEngine.Entry(entryID)
	nextRun := nextEntry.Next
	schedule.NextRun = &nextRun
	s.db.Save(schedule)

	log.Info().
		Uint("schedule_id", schedule.ID).
		Str("schedule_name", schedule.Name).
		Str("cron", schedule.CronExpr).
		Str("timezone", schedule.Timezone).
		Time("next_run", nextRun).
		Msg("Schedule registered successfully")

	return nil
}

// executeSchedule executes a scheduled task with overlap prevention and timeout
func (s *SchedulerService) executeSchedule(schedule *models.PluginSchedule) {
	instanceID := s.leaderElection.GetInstanceID()

	// CHECK 1: Is plugin still active?
	var plugin models.Plugin
	if err := s.db.First(&plugin, schedule.PluginID).Error; err != nil {
		log.Debug().
			Uint("plugin_id", schedule.PluginID).
			Uint("schedule_id", schedule.ID).
			Msg("Plugin not found for schedule, skipping execution")
		return
	}

	if !plugin.IsActive {
		log.Debug().
			Uint("plugin_id", schedule.PluginID).
			Uint("schedule_id", schedule.ID).
			Msg("Plugin is not active, skipping schedule execution")
		return
	}

	// CHECK 2: Is there already a running execution for this schedule?
	var runningExec models.PluginScheduleExecution
	result := s.db.Where("plugin_schedule_id = ? AND status = ?", schedule.ID, "running").First(&runningExec)

	if result.Error == nil {
		log.Debug().
			Uint("schedule_id", schedule.ID).
			Uint("execution_id", runningExec.ID).
			Msg("Schedule already running, skipping to prevent overlap")
		return
	}

	// CREATE execution record with "pending" status
	execution := &models.PluginScheduleExecution{
		PluginScheduleID: schedule.ID,
		PluginID:         schedule.PluginID,
		Status:           "pending",
		StartedAt:        time.Now(),
		LockedBy:         instanceID,
	}
	s.db.Create(execution)

	// UPDATE to "running" status (atomic)
	result = s.db.Model(execution).Where("id = ? AND status = ?", execution.ID, "pending").
		Update("status", "running")

	if result.RowsAffected == 0 {
		log.Debug().
			Uint("execution_id", execution.ID).
			Msg("Failed to lock execution (race condition)")
		return
	}

	log.Info().
		Uint("schedule_id", schedule.ID).
		Str("schedule_name", schedule.Name).
		Uint("plugin_id", schedule.PluginID).
		Int("timeout_seconds", schedule.TimeoutSeconds).
		Msg("Executing scheduled task")

	// Execute with timeout
	timeout := time.Duration(schedule.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	errChan := make(chan error, 1)

	go func() {
		errChan <- s.callPluginScheduleTask(ctx, &plugin, schedule)
	}()

	var execErr error
	select {
	case execErr = <-errChan:
		// Completed normally
	case <-ctx.Done():
		// Timeout
		execErr = fmt.Errorf("execution timed out after %d seconds", schedule.TimeoutSeconds)
		execution.Status = "timeout"
	}

	// Record result
	now := time.Now()
	execution.CompletedAt = &now
	execution.Duration = time.Since(start).Milliseconds()

	if execErr != nil {
		execution.Success = false
		execution.Error = execErr.Error()
		if execution.Status != "timeout" {
			execution.Status = "failed"
		}
		log.Error().
			Err(execErr).
			Uint("schedule_id", schedule.ID).
			Str("schedule_name", schedule.Name).
			Dur("duration", time.Since(start)).
			Msg("Scheduled task execution failed")
	} else {
		execution.Success = true
		execution.Status = "completed"
		log.Info().
			Uint("schedule_id", schedule.ID).
			Str("schedule_name", schedule.Name).
			Dur("duration", time.Since(start)).
			Msg("Scheduled task execution completed successfully")
	}

	s.db.Save(execution)

	// Update schedule last run
	schedule.LastRun = &now
	s.db.Save(schedule)
}

// callPluginScheduleTask calls the plugin's ExecuteScheduledTask via gRPC
func (s *SchedulerService) callPluginScheduleTask(ctx context.Context, plugin *models.Plugin, schedule *models.PluginSchedule) error {
	// Build gRPC request
	scheduleProto := &pb.ScheduleDefinition{
		Id:             schedule.ManifestScheduleID,
		Name:           schedule.Name,
		Cron:           schedule.CronExpr,
		Timezone:       schedule.Timezone,
		Enabled:        schedule.Enabled,
		TimeoutSeconds: int32(schedule.TimeoutSeconds),
		ConfigJson:     schedule.Config, // Already JSON string from database
	}

	contextProto := &pb.PluginContext{
		RequestId: fmt.Sprintf("schedule-%d-%d", schedule.ID, time.Now().Unix()),
	}

	// Execute via plugin client
	resp, err := s.pluginClient.ExecuteScheduledTask(ctx, plugin.ID, contextProto, scheduleProto)

	if err != nil {
		return fmt.Errorf("gRPC error: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("plugin returned error: %s", resp.ErrorMessage)
	}

	return nil
}

// RegisterSchedule registers a new schedule (called when plugin is installed)
func (s *SchedulerService) RegisterSchedule(schedule *models.PluginSchedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we're the leader
	isLeader, err := s.leaderElection.IsLeader()
	if err != nil {
		return fmt.Errorf("failed to check leadership: %w", err)
	}

	if !isLeader {
		// Not the leader, don't register locally (leader will pick it up on next sync)
		return nil
	}

	return s.registerScheduleUnsafe(schedule)
}

// UnregisterSchedule removes a schedule (called when plugin is uninstalled)
func (s *SchedulerService) UnregisterSchedule(scheduleID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	runner, exists := s.schedules[scheduleID]
	if !exists {
		return nil // Not registered
	}

	// Stop cron engine
	if runner.cronEngine != nil {
		runner.cronEngine.Stop()
	}

	// Remove from map
	delete(s.schedules, scheduleID)

	log.Info().
		Uint("schedule_id", scheduleID).
		Msg("Schedule unregistered")
	return nil
}

// Helper function to convert map to JSON
func mapToJSON(m map[string]interface{}) string {
	if len(m) == 0 {
		return "{}"
	}

	bytes, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}

	return string(bytes)
}
