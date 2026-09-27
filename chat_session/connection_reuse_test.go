package chat_session

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestPostgreSQLConnectionReuse verifies actual connection reuse with real PostgreSQL
func TestPostgreSQLConnectionReuse(t *testing.T) {
	// Skip if no DATABASE_URL provided
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL connection reuse test")
	}

	// Create a shared database connection with limited pool
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}

	// Configure a small connection pool to test limits
	sqlDB.SetMaxOpenConns(3)  // Very small limit to test sharing
	sqlDB.SetMaxIdleConns(1)

	initialStats := sqlDB.Stats()
	t.Logf("Initial connection stats: Open=%d, InUse=%d, Idle=%d, MaxOpen=%d",
		initialStats.OpenConnections, initialStats.InUse, initialStats.Idle, initialStats.MaxOpenConnections)

	// Test configuration
	cfg := config.QueueConfig{
		Type:       "postgres",
		BufferSize: 50,
		PostgreSQL: config.PostgreSQLQueueConfig{
			ReconnectInterval:   "1s",
			MaxReconnectRetries: 3,
			NotifyTimeout:       "2s",
		},
	}

	// Create shared factory
	factory, err := CreateQueueFactoryWithSharedDB(cfg, db)
	if err != nil {
		t.Fatalf("Failed to create shared queue factory: %v", err)
	}

	// Simulate creating multiple chat sessions
	sessionCount := 5 // This would create 125 connections with the old implementation (5 * 25)
	queues := make([]MessageQueue, 0)

	t.Logf("Creating %d queue instances (would have been %d connections before fix)...", sessionCount, sessionCount*25)

	for i := 0; i < sessionCount; i++ {
		sessionID := fmt.Sprintf("test-session-%d", i)

		// Each session used to pin a pool connection, so the fourth creation
		// blocked forever on this 3-connection pool; fail instead of hanging.
		queue, err := createQueueWithin(t, factory, sessionID, 15*time.Second)
		if err != nil {
			t.Fatalf("Queue %d creation failed: %v", i+1, err)
		}
		
		queues = append(queues, queue)
		t.Logf("✅ Queue %d created successfully", i+1)
	}

	// Check final connection stats
	finalStats := sqlDB.Stats()
	t.Logf("Final connection stats: Open=%d, InUse=%d, Idle=%d",
		finalStats.OpenConnections, finalStats.InUse, finalStats.Idle)

	// The key test: we should not exceed our MaxOpenConns limit
	if finalStats.OpenConnections > finalStats.MaxOpenConnections {
		t.Errorf("Connection limit exceeded: %d > %d", finalStats.OpenConnections, finalStats.MaxOpenConnections)
	}

	// With the fix, we should stay within reasonable limits
	if finalStats.OpenConnections <= 3 { // Our MaxOpenConns limit
		t.Logf("✅ Connection reuse working: stayed within limit of %d connections", finalStats.MaxOpenConnections)
	} else {
		t.Errorf("❌ Too many connections: %d (should be ≤ %d)", finalStats.OpenConnections, finalStats.MaxOpenConnections)
	}

	// Cleanup
	for i, queue := range queues {
		if queue != nil {
			queue.Close()
			t.Logf("Closed queue %d", i+1)
		}
	}

	t.Log("🎉 PostgreSQL connection reuse test completed")
}