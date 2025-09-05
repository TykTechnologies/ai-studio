package chat_session

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgreSQLQueue_BasicOperations(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL queue tests")
	}

	// Connect to PostgreSQL database
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	// Test database connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// Create PostgreSQL queue
	sessionID := "test-session-" + fmt.Sprint(time.Now().Unix())
	config := DefaultPostgreSQLConfig()
	config.BufferSize = 10 // Small buffer for testing
	
	queue, err := NewPostgreSQLQueue(sessionID, db, config)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL queue: %v", err)
	}
	defer queue.Close()

	// Test context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test publishing and consuming messages
	testMessage := &ChatResponse{
		Payload: "Test PostgreSQL message",
	}

	// Publish message
	if err := queue.PublishMessage(ctx, testMessage); err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}

	// Consume message
	messageChan := queue.ConsumeMessages(ctx)
	
	select {
	case receivedMsg := <-messageChan:
		if receivedMsg.Payload != testMessage.Payload {
			t.Errorf("Expected payload %q, got %q", testMessage.Payload, receivedMsg.Payload)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

func TestPostgreSQLQueue_StreamData(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL queue tests")
	}

	// Connect to PostgreSQL database
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	// Test database connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// Create PostgreSQL queue
	sessionID := "test-stream-" + fmt.Sprint(time.Now().Unix())
	config := DefaultPostgreSQLConfig()
	
	queue, err := NewPostgreSQLQueue(sessionID, db, config)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL queue: %v", err)
	}
	defer queue.Close()

	// Test context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test stream data
	testData := []byte("test stream data")

	// Publish stream data
	if err := queue.PublishStream(ctx, testData); err != nil {
		t.Fatalf("Failed to publish stream data: %v", err)
	}

	// Consume stream data
	streamChan := queue.ConsumeStream(ctx)
	
	select {
	case receivedData := <-streamChan:
		if string(receivedData) != string(testData) {
			t.Errorf("Expected stream data %q, got %q", string(testData), string(receivedData))
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for stream data")
	}
}

func TestPostgreSQLQueue_Error(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL queue tests")
	}

	// Connect to PostgreSQL database  
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	// Test database connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// Create PostgreSQL queue
	sessionID := "test-error-" + fmt.Sprint(time.Now().Unix())
	config := DefaultPostgreSQLConfig()
	
	queue, err := NewPostgreSQLQueue(sessionID, db, config)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL queue: %v", err)
	}
	defer queue.Close()

	// Test context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test error
	testError := fmt.Errorf("test PostgreSQL error")

	// Publish error
	if err := queue.PublishError(ctx, testError); err != nil {
		t.Fatalf("Failed to publish error: %v", err)
	}

	// Consume error
	errorChan := queue.ConsumeErrors(ctx)
	
	select {
	case receivedError := <-errorChan:
		if receivedError.Error() != testError.Error() {
			t.Errorf("Expected error %q, got %q", testError.Error(), receivedError.Error())
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for error")
	}
}

func TestPostgreSQLQueue_Close(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL queue tests")
	}

	// Connect to PostgreSQL database
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	// Test database connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// Create PostgreSQL queue
	sessionID := "test-close-" + fmt.Sprint(time.Now().Unix())
	config := DefaultPostgreSQLConfig()
	
	queue, err := NewPostgreSQLQueue(sessionID, db, config)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL queue: %v", err)
	}

	// Test queue depth before close
	messages, stream, errors, llmResponses := queue.QueueDepth()
	if messages != 0 || stream != 0 || errors != 0 || llmResponses != 0 {
		t.Errorf("Expected empty queue depths, got messages=%d, stream=%d, errors=%d, llmResponses=%d", 
			messages, stream, errors, llmResponses)
	}

	// Close queue
	if err := queue.Close(); err != nil {
		t.Fatalf("Failed to close queue: %v", err)
	}

	// Verify queue is closed
	messages, stream, errors, llmResponses = queue.QueueDepth()
	if messages != 0 || stream != 0 || errors != 0 || llmResponses != 0 {
		t.Errorf("Expected zero queue depths after close, got messages=%d, stream=%d, errors=%d, llmResponses=%d", 
			messages, stream, errors, llmResponses)
	}

	// Test that publishing fails after close
	ctx := context.Background()
	testMessage := &ChatResponse{
		Payload: "Test message after close",
	}
	
	if err := queue.PublishMessage(ctx, testMessage); err == nil {
		t.Error("Expected error when publishing to closed queue, but got nil")
	}
}

func TestPostgreSQLQueueFactory(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL") 
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL queue factory tests")
	}

	// Connect to PostgreSQL database
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}

	// Test database connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not accessible: %v", err)
	}

	// Create PostgreSQL queue factory
	config := DefaultPostgreSQLConfig()
	factory := NewPostgreSQLQueueFactory(db, config)

	sessionID := "test-factory-" + fmt.Sprint(time.Now().Unix())
	
	// Test queue creation
	queue, err := factory.CreateQueue(sessionID, nil)
	if err != nil {
		t.Fatalf("Failed to create queue from factory: %v", err)
	}
	defer queue.Close()

	// Test queue creation with custom config
	customConfig := map[string]interface{}{
		"bufferSize": 50,
	}
	
	queue2, err := factory.CreateQueue(sessionID+"_custom", customConfig)
	if err != nil {
		t.Fatalf("Failed to create queue with custom config: %v", err)
	}
	defer queue2.Close()

	// Verify the queues work
	ctx := context.Background()
	testMessage := &ChatResponse{
		Payload: "Factory test message",
	}

	// Test first queue
	if err := queue.PublishMessage(ctx, testMessage); err != nil {
		t.Fatalf("Failed to publish message to factory-created queue: %v", err)
	}

	// Test second queue  
	if err := queue2.PublishMessage(ctx, testMessage); err != nil {
		t.Fatalf("Failed to publish message to custom-config queue: %v", err)
	}
}