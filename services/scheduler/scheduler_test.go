package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockPluginClient implements PluginClient for testing
type mockPluginClient struct {
	executeScheduledTaskFunc func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error)
	callCount                int
}

func (m *mockPluginClient) ExecuteScheduledTask(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
	m.callCount++
	if m.executeScheduledTaskFunc != nil {
		return m.executeScheduledTaskFunc(ctx, pluginID, contextProto, scheduleProto)
	}
	return &pb.ExecuteScheduledTaskResponse{Success: true}, nil
}

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate tables
	err = db.AutoMigrate(
		&models.Plugin{},
		&models.PluginSchedule{},
		&models.PluginScheduleExecution{},
		&models.SchedulerLease{},
	)
	require.NoError(t, err)

	return db
}

// TestLeaderElection tests leader election logic
func TestLeaderElection(t *testing.T) {
	db := setupTestDB(t)

	// Create two leader election managers (simulating two instances)
	lem1 := NewLeaderElectionManager(db)
	lem2 := NewLeaderElectionManager(db)

	// Both should have different instance IDs
	assert.NotEqual(t, lem1.GetInstanceID(), lem2.GetInstanceID())

	// First instance should become leader
	isLeader1, err := lem1.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader1, "First instance should become leader")

	// Second instance should NOT become leader (lease not expired)
	isLeader2, err := lem2.TryBecomeLeader()
	require.NoError(t, err)
	assert.False(t, isLeader2, "Second instance should not become leader while lease is valid")

	// First instance should remain leader on renewal
	isLeader1Again, err := lem1.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader1Again, "Leader should successfully renew lease")

	// Verify leader in database
	var lease models.SchedulerLease
	err = db.First(&lease, 1).Error
	require.NoError(t, err)
	assert.Equal(t, lem1.GetInstanceID(), lease.LeaderID)
	assert.True(t, lease.ExpiresAt.After(time.Now()), "Lease should not be expired")
}

// TestLeaderElectionExpiration tests that leadership changes when lease expires
func TestLeaderElectionExpiration(t *testing.T) {
	db := setupTestDB(t)

	lem1 := NewLeaderElectionManager(db)
	lem2 := NewLeaderElectionManager(db)

	// Instance 1 becomes leader
	isLeader, err := lem1.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, isLeader)

	// Manually expire the lease
	var lease models.SchedulerLease
	db.First(&lease, 1)
	lease.ExpiresAt = time.Now().Add(-1 * time.Minute)
	db.Save(&lease)

	// Instance 2 should now be able to become leader
	isLeader2, err := lem2.TryBecomeLeader()
	require.NoError(t, err)
	assert.True(t, isLeader2, "New instance should become leader after lease expires")

	// Verify new leader
	db.First(&lease, 1)
	assert.Equal(t, lem2.GetInstanceID(), lease.LeaderID)
}

// TestScheduleExecution tests basic schedule execution
func TestScheduleExecution(t *testing.T) {
	db := setupTestDB(t)

	// Create a test plugin
	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: true,
	}
	db.Create(&plugin)

	// Create a test schedule
	schedule := models.PluginSchedule{
		PluginID:           plugin.ID,
		ManifestScheduleID: "test-task",
		Name:               "Test Task",
		CronExpr:           "* * * * *",
		Timezone:           "UTC",
		Enabled:            true,
		TimeoutSeconds:     60,
		Config:             "{}",
	}
	db.Create(&schedule)

	// Create mock plugin client
	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			assert.Equal(t, plugin.ID, pluginID)
			assert.Equal(t, "test-task", scheduleProto.Id)
			assert.Equal(t, "Test Task", scheduleProto.Name)
			return &pb.ExecuteScheduledTaskResponse{Success: true}, nil
		},
	}

	// Create scheduler service
	schedulerService := NewSchedulerService(db, mockClient)

	// Execute the schedule
	schedulerService.executeSchedule(&schedule)

	// Verify execution was recorded
	var execution models.PluginScheduleExecution
	err := db.Where("plugin_schedule_id = ?", schedule.ID).First(&execution).Error
	require.NoError(t, err)

	assert.Equal(t, "completed", execution.Status)
	assert.True(t, execution.Success)
	assert.Empty(t, execution.Error)
	assert.Greater(t, execution.Duration, int64(0))

	// Verify plugin client was called
	assert.Equal(t, 1, mockClient.callCount)
}

// TestScheduleExecutionFailure tests error handling
func TestScheduleExecutionFailure(t *testing.T) {
	db := setupTestDB(t)

	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: true,
	}
	db.Create(&plugin)

	schedule := models.PluginSchedule{
		PluginID:           plugin.ID,
		ManifestScheduleID: "failing-task",
		Name:               "Failing Task",
		CronExpr:           "* * * * *",
		Timezone:           "UTC",
		Enabled:            true,
		TimeoutSeconds:     60,
		Config:             "{}",
	}
	db.Create(&schedule)

	// Mock plugin client that returns error
	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			return nil, fmt.Errorf("plugin execution failed")
		},
	}

	schedulerService := NewSchedulerService(db, mockClient)
	schedulerService.executeSchedule(&schedule)

	// Verify failure was recorded
	var execution models.PluginScheduleExecution
	err := db.Where("plugin_schedule_id = ?", schedule.ID).First(&execution).Error
	require.NoError(t, err)

	assert.Equal(t, "failed", execution.Status)
	assert.False(t, execution.Success)
	assert.Contains(t, execution.Error, "plugin execution failed")
}

// TestScheduleExecutionTimeout tests timeout handling
func TestScheduleExecutionTimeout(t *testing.T) {
	db := setupTestDB(t)

	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: true,
	}
	db.Create(&plugin)

	// Create schedule with very short timeout
	schedule := models.PluginSchedule{
		PluginID:       plugin.ID,
		ManifestScheduleID: "slow-task",
		Name:           "Slow Task",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 1, // 1 second timeout
		Config:         "{}",
	}
	db.Create(&schedule)

	// Mock plugin client that takes longer than timeout
	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			time.Sleep(2 * time.Second) // Exceed timeout
			return &pb.ExecuteScheduledTaskResponse{Success: true}, nil
		},
	}

	schedulerService := NewSchedulerService(db, mockClient)
	schedulerService.executeSchedule(&schedule)

	// Verify timeout was recorded
	var execution models.PluginScheduleExecution
	err := db.Where("plugin_schedule_id = ?", schedule.ID).First(&execution).Error
	require.NoError(t, err)

	assert.Equal(t, "timeout", execution.Status)
	assert.False(t, execution.Success)
	assert.Contains(t, execution.Error, "timed out")
}

// TestOverlapPrevention tests that overlapping executions are prevented
func TestOverlapPrevention(t *testing.T) {
	db := setupTestDB(t)

	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: true,
	}
	db.Create(&plugin)

	schedule := models.PluginSchedule{
		PluginID:       plugin.ID,
		ManifestScheduleID: "long-task",
		Name:           "Long Running Task",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 10,
		Config:         "{}",
	}
	db.Create(&schedule)

	// Create a "running" execution to simulate overlap
	runningExecution := models.PluginScheduleExecution{
		PluginScheduleID: schedule.ID,
		PluginID:         plugin.ID,
		Status:           "running",
		StartedAt:        time.Now(),
		LockedBy:         "test-instance-1",
	}
	db.Create(&runningExecution)

	// Mock client should NOT be called
	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			t.Fatal("Plugin should not be called when execution is already running")
			return nil, nil
		},
	}

	schedulerService := NewSchedulerService(db, mockClient)
	schedulerService.executeSchedule(&schedule)

	// Verify no new execution was created
	var executions []models.PluginScheduleExecution
	db.Where("plugin_schedule_id = ?", schedule.ID).Find(&executions)
	assert.Equal(t, 1, len(executions), "Should only have the original running execution")
	assert.Equal(t, 0, mockClient.callCount, "Plugin should not have been called")
}

// TestInactivePluginSkipped tests that inactive plugins are skipped
func TestInactivePluginSkipped(t *testing.T) {
	db := setupTestDB(t)

	// Create inactive plugin
	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: false, // INACTIVE
	}
	db.Create(&plugin)

	schedule := models.PluginSchedule{
		PluginID:       plugin.ID,
		ManifestScheduleID: "test-task",
		Name:           "Test Task",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 60,
		Config:         "{}",
	}
	db.Create(&schedule)

	// Mock client should NOT be called
	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			t.Fatal("Plugin should not be called when inactive")
			return nil, nil
		},
	}

	schedulerService := NewSchedulerService(db, mockClient)
	schedulerService.executeSchedule(&schedule)

	// Verify no execution was created
	var count int64
	db.Model(&models.PluginScheduleExecution{}).Where("plugin_schedule_id = ?", schedule.ID).Count(&count)
	assert.Equal(t, int64(0), count, "No execution should be created for inactive plugin")
	assert.Equal(t, 0, mockClient.callCount, "Plugin should not have been called")
}

// TestCronExpressionParsing tests various cron expressions
func TestCronExpressionParsing(t *testing.T) {
	db := setupTestDB(t)
	mockClient := &mockPluginClient{}
	schedulerService := NewSchedulerService(db, mockClient)

	testCases := []struct {
		name       string
		cronExpr   string
		shouldFail bool
	}{
		{"Every minute", "* * * * *", false},
		{"Every hour", "0 * * * *", false},
		{"Daily at midnight", "0 0 * * *", false},
		{"Every 5 minutes", "*/5 * * * *", false},
		{"With seconds", "0 * * * * *", false},
		{"Invalid expression", "invalid", true},
		{"Too many fields", "* * * * * * *", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			schedule := &models.PluginSchedule{
				PluginID:       1,
				ManifestScheduleID: fmt.Sprintf("test-%s", tc.name),
				Name:           tc.name,
				CronExpr:       tc.cronExpr,
				Timezone:       "UTC",
				Enabled:        true,
				TimeoutSeconds: 60,
				Config:         "{}",
			}

			err := schedulerService.registerScheduleUnsafe(schedule)

			if tc.shouldFail {
				assert.Error(t, err, "Should fail for invalid cron expression")
			} else {
				assert.NoError(t, err, "Should succeed for valid cron expression")
			}
		})
	}
}

// TestTimezoneSupport tests timezone configuration
func TestTimezoneSupport(t *testing.T) {
	db := setupTestDB(t)
	mockClient := &mockPluginClient{}
	schedulerService := NewSchedulerService(db, mockClient)

	testCases := []struct {
		timezone string
		isValid  bool
	}{
		{"UTC", true},
		{"America/New_York", true},
		{"Europe/London", true},
		{"Asia/Tokyo", true},
		{"Invalid/Timezone", false},
	}

	for _, tc := range testCases {
		t.Run(tc.timezone, func(t *testing.T) {
			schedule := &models.PluginSchedule{
				PluginID:       1,
				ManifestScheduleID: fmt.Sprintf("test-%s", tc.timezone),
				Name:           "Test Task",
				CronExpr:       "0 9 * * *", // 9 AM
				Timezone:       tc.timezone,
				Enabled:        true,
				TimeoutSeconds: 60,
				Config:         "{}",
			}

			err := schedulerService.registerScheduleUnsafe(schedule)

			if tc.isValid {
				assert.NoError(t, err, "Should accept valid timezone")
				// Verify schedule was registered
				_, exists := schedulerService.schedules[schedule.ID]
				assert.True(t, exists, "Schedule should be registered")
			} else {
				// Invalid timezone should still register but log warning (uses UTC fallback)
				assert.NoError(t, err, "Should register with UTC fallback for invalid timezone")
			}
		})
	}
}

// TestExecutionHistory tests that execution history is properly recorded
func TestExecutionHistory(t *testing.T) {
	db := setupTestDB(t)

	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: true,
	}
	db.Create(&plugin)

	schedule := models.PluginSchedule{
		PluginID:       plugin.ID,
		ManifestScheduleID: "history-test",
		Name:           "History Test",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 60,
		Config:         "{}",
	}
	db.Create(&schedule)

	mockClient := &mockPluginClient{}
	schedulerService := NewSchedulerService(db, mockClient)

	// Execute multiple times
	for i := 0; i < 5; i++ {
		schedulerService.executeSchedule(&schedule)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Verify 5 executions were recorded
	var executions []models.PluginScheduleExecution
	db.Where("plugin_schedule_id = ?", schedule.ID).Order("started_at ASC").Find(&executions)
	assert.Equal(t, 5, len(executions))

	// Verify all executions have required fields
	for i, exec := range executions {
		assert.Equal(t, schedule.ID, exec.PluginScheduleID, "Execution %d: schedule ID mismatch", i)
		assert.Equal(t, plugin.ID, exec.PluginID, "Execution %d: plugin ID mismatch", i)
		assert.Equal(t, "completed", exec.Status, "Execution %d: should be completed", i)
		assert.True(t, exec.Success, "Execution %d: should be successful", i)
		assert.NotNil(t, exec.CompletedAt, "Execution %d: should have completion time", i)
		assert.Greater(t, exec.Duration, int64(0), "Execution %d: should have duration", i)
	}

	// Verify schedule last_run was updated
	var updatedSchedule models.PluginSchedule
	db.First(&updatedSchedule, schedule.ID)
	assert.NotNil(t, updatedSchedule.LastRun)
}

// TestPluginNotFoundHandling tests handling of missing plugins
func TestPluginNotFoundHandling(t *testing.T) {
	db := setupTestDB(t)

	// Create schedule for non-existent plugin
	schedule := models.PluginSchedule{
		PluginID:       999, // Non-existent
		ManifestScheduleID: "orphan-task",
		Name:           "Orphan Task",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 60,
		Config:         "{}",
	}
	db.Create(&schedule)

	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			t.Fatal("Should not call plugin that doesn't exist")
			return nil, nil
		},
	}

	schedulerService := NewSchedulerService(db, mockClient)
	schedulerService.executeSchedule(&schedule)

	// Verify no execution was created
	var count int64
	db.Model(&models.PluginScheduleExecution{}).Where("plugin_schedule_id = ?", schedule.ID).Count(&count)
	assert.Equal(t, int64(0), count)
	assert.Equal(t, 0, mockClient.callCount)
}

// TestScheduleRegistrationAndUnregistration tests lifecycle management
func TestScheduleRegistrationAndUnregistration(t *testing.T) {
	db := setupTestDB(t)
	mockClient := &mockPluginClient{}

	// Become leader first
	schedulerService := NewSchedulerService(db, mockClient)
	isLeader, err := schedulerService.leaderElection.TryBecomeLeader()
	require.NoError(t, err)
	require.True(t, isLeader)

	// Register a schedule
	schedule := &models.PluginSchedule{
		ID:             1,
		PluginID:       1,
		ManifestScheduleID: "reg-test",
		Name:           "Registration Test",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 60,
		Config:         "{}",
	}

	err = schedulerService.RegisterSchedule(schedule)
	require.NoError(t, err)

	// Verify schedule is registered
	schedulerService.mu.RLock()
	_, exists := schedulerService.schedules[schedule.ID]
	schedulerService.mu.RUnlock()
	assert.True(t, exists, "Schedule should be registered")

	// Unregister schedule
	err = schedulerService.UnregisterSchedule(schedule.ID)
	require.NoError(t, err)

	// Verify schedule is unregistered
	schedulerService.mu.RLock()
	_, exists = schedulerService.schedules[schedule.ID]
	schedulerService.mu.RUnlock()
	assert.False(t, exists, "Schedule should be unregistered")
}

// TestConfigParsing tests schedule config JSON parsing
func TestConfigParsing(t *testing.T) {
	db := setupTestDB(t)

	plugin := models.Plugin{
		Name:     "Test Plugin",
		Command:  "/test/plugin",
		HookType: "studio_ui",
		IsActive: true,
	}
	db.Create(&plugin)

	// Schedule with JSON config
	configJSON := `{"batch_size": 100, "api_key": "test-key", "enabled": true}`
	schedule := models.PluginSchedule{
		PluginID:       plugin.ID,
		ManifestScheduleID: "config-test",
		Name:           "Config Test",
		CronExpr:       "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		TimeoutSeconds: 60,
		Config:         configJSON,
	}
	db.Create(&schedule)

	mockClient := &mockPluginClient{
		executeScheduledTaskFunc: func(ctx context.Context, pluginID uint, contextProto *pb.PluginContext, scheduleProto *pb.ScheduleDefinition) (*pb.ExecuteScheduledTaskResponse, error) {
			// Verify config is passed correctly
			assert.Equal(t, configJSON, scheduleProto.ConfigJson)
			return &pb.ExecuteScheduledTaskResponse{Success: true}, nil
		},
	}

	schedulerService := NewSchedulerService(db, mockClient)
	schedulerService.executeSchedule(&schedule)

	assert.Equal(t, 1, mockClient.callCount, "Plugin should have been called")
}
