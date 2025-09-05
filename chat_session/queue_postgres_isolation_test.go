package chat_session

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestPostgreSQLQueue_MultiUserIsolation validates that messages from different users/sessions
// are properly isolated and don't interfere with each other
func TestPostgreSQLQueue_MultiUserIsolation(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL multi-user isolation tests")
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

	// Create multiple users with different session IDs
	users := []struct {
		name      string
		sessionID string
	}{
		{"user1", "session-user1-" + fmt.Sprint(time.Now().Unix())},
		{"user2", "session-user2-" + fmt.Sprint(time.Now().Unix())},
		{"user3", "session-user3-" + fmt.Sprint(time.Now().Unix())},
	}

	// Create queues for each user
	queues := make([]*PostgreSQLQueue, len(users))
	config := DefaultPostgreSQLConfig()
	config.BufferSize = 10

	for i, user := range users {
		queue, err := NewPostgreSQLQueue(user.sessionID, db, config)
		if err != nil {
			t.Fatalf("Failed to create PostgreSQL queue for %s: %v", user.name, err)
		}
		queues[i] = queue
		defer queue.Close()
	}

	// Test context
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test 1: Each user sends a unique message
	messages := make(map[string]*ChatResponse)
	for i, user := range users {
		msg := &ChatResponse{
			Payload: fmt.Sprintf("Message from %s at %d", user.name, time.Now().UnixNano()),
		}
		messages[user.sessionID] = msg

		if err := queues[i].PublishMessage(ctx, msg); err != nil {
			t.Fatalf("Failed to publish message for %s: %v", user.name, err)
		}
	}

	// Test 2: Each user should only receive their own message
	var wg sync.WaitGroup
	receivedMessages := make(map[string]*ChatResponse)
	var mu sync.Mutex

	for i, user := range users {
		wg.Add(1)
		go func(idx int, userName, sessionID string) {
			defer wg.Done()

			messageChan := queues[idx].ConsumeMessages(ctx)
			select {
			case receivedMsg := <-messageChan:
				mu.Lock()
				receivedMessages[sessionID] = receivedMsg
				mu.Unlock()
				t.Logf("%s received message: %s", userName, receivedMsg.Payload)
			case <-time.After(5 * time.Second):
				t.Errorf("%s did not receive message within timeout", userName)
			}
		}(i, user.name, user.sessionID)
	}

	// Wait for all messages to be received
	wg.Wait()

	// Validate that each user received exactly their own message
	for sessionID, originalMsg := range messages {
		receivedMsg, ok := receivedMessages[sessionID]
		if !ok {
			t.Errorf("Session %s did not receive any message", sessionID)
			continue
		}

		if receivedMsg.Payload != originalMsg.Payload {
			t.Errorf("Session %s received wrong message. Expected: %q, Got: %q",
				sessionID, originalMsg.Payload, receivedMsg.Payload)
		}
	}

	// Validate no cross-contamination - each user should have received exactly 1 message
	if len(receivedMessages) != len(users) {
		t.Errorf("Expected %d received messages, got %d", len(users), len(receivedMessages))
	}
}

// TestPostgreSQLQueue_ConcurrentMessageIsolation tests high-concurrency scenarios
func TestPostgreSQLQueue_ConcurrentMessageIsolation(t *testing.T) {
	// Skip test if no PostgreSQL connection available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL concurrent isolation tests")
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

	const numUsers = 5
	const messagesPerUser = 10

	// Create multiple users
	users := make([]struct {
		name      string
		sessionID string
		queue     *PostgreSQLQueue
	}, numUsers)

	config := DefaultPostgreSQLConfig()
	config.BufferSize = 20

	for i := 0; i < numUsers; i++ {
		sessionID := fmt.Sprintf("concurrent-session-%d-%d", i, time.Now().UnixNano())
		queue, err := NewPostgreSQLQueue(sessionID, db, config)
		if err != nil {
			t.Fatalf("Failed to create PostgreSQL queue for user %d: %v", i, err)
		}
		defer queue.Close()

		users[i] = struct {
			name      string
			sessionID string
			queue     *PostgreSQLQueue
		}{
			name:      fmt.Sprintf("user%d", i),
			sessionID: sessionID,
			queue:     queue,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Each user sends multiple messages concurrently
	var publishWG sync.WaitGroup
	expectedMessages := make(map[string][]string) // sessionID -> list of messages
	var expectedMu sync.Mutex

	for userIdx, user := range users {
		publishWG.Add(1)
		go func(idx int, u struct {
			name      string
			sessionID string
			queue     *PostgreSQLQueue
		}) {
			defer publishWG.Done()

			userMessages := make([]string, messagesPerUser)
			for msgIdx := 0; msgIdx < messagesPerUser; msgIdx++ {
				msgContent := fmt.Sprintf("Message %d from %s", msgIdx, u.name)
				userMessages[msgIdx] = msgContent

				msg := &ChatResponse{Payload: msgContent}
				if err := u.queue.PublishMessage(ctx, msg); err != nil {
					t.Errorf("Failed to publish message for %s: %v", u.name, err)
					return
				}
				
				// Small delay to interleave messages from different users
				time.Sleep(1 * time.Millisecond)
			}

			expectedMu.Lock()
			expectedMessages[u.sessionID] = userMessages
			expectedMu.Unlock()
		}(userIdx, user)
	}

	// Wait for all publishing to complete
	publishWG.Wait()

	// Each user receives their messages
	var receiveWG sync.WaitGroup
	receivedMessages := make(map[string][]string) // sessionID -> received messages
	var receivedMu sync.Mutex

	for _, user := range users {
		receiveWG.Add(1)
		go func(u struct {
			name      string
			sessionID string
			queue     *PostgreSQLQueue
		}) {
			defer receiveWG.Done()

			messageChan := u.queue.ConsumeMessages(ctx)
			received := make([]string, 0, messagesPerUser)

			for i := 0; i < messagesPerUser; i++ {
				select {
				case msg := <-messageChan:
					received = append(received, msg.Payload)
				case <-time.After(5 * time.Second):
					t.Errorf("%s: timeout waiting for message %d", u.name, i)
					return
				}
			}

			receivedMu.Lock()
			receivedMessages[u.sessionID] = received
			receivedMu.Unlock()
		}(user)
	}

	// Wait for all receiving to complete
	receiveWG.Wait()

	// Validate message isolation - each user should receive exactly their own messages
	for sessionID, expected := range expectedMessages {
		received, ok := receivedMessages[sessionID]
		if !ok {
			t.Errorf("Session %s received no messages", sessionID)
			continue
		}

		if len(received) != len(expected) {
			t.Errorf("Session %s: expected %d messages, received %d",
				sessionID, len(expected), len(received))
			continue
		}

		// Check that all expected messages were received (order may vary due to concurrency)
		expectedSet := make(map[string]bool)
		for _, msg := range expected {
			expectedSet[msg] = true
		}

		for _, msg := range received {
			if !expectedSet[msg] {
				t.Errorf("Session %s received unexpected message: %q", sessionID, msg)
			} else {
				delete(expectedSet, msg) // Mark as received
			}
		}

		// Check for any missing messages
		for msg := range expectedSet {
			t.Errorf("Session %s missing expected message: %q", sessionID, msg)
		}
	}

	t.Logf("✅ Successfully tested %d users sending %d messages each concurrently with perfect isolation",
		numUsers, messagesPerUser)
}

// TestPostgreSQLQueue_ChannelNameIsolation validates the channel naming mechanism
func TestPostgreSQLQueue_ChannelNameIsolation(t *testing.T) {
	// Test that different sessions generate different channel names
	sessionIDs := []string{
		"session-1",
		"session-2", 
		"room-abc-user-123",
		"room-xyz-user-456",
	}

	channels := make(map[string]bool)
	
	for _, sessionID := range sessionIDs {
		// Create a temporary queue just to test channel naming
		psq := &PostgreSQLQueue{sessionID: sessionID}
		
		// Test all message types
		messageTypes := []string{
			PostgreSQLMessageTypeChatResponse,
			PostgreSQLMessageTypeStream,
			PostgreSQLMessageTypeError,
			PostgreSQLMessageTypeLLMResponse,
		}
		
		for _, msgType := range messageTypes {
			channelName := psq.getChannelName(msgType)
			
			// Ensure channel name includes session ID
			expectedChannel := fmt.Sprintf("chat_%s_%s", msgType, sessionID)
			if channelName != expectedChannel {
				t.Errorf("Wrong channel name. Expected: %s, Got: %s", expectedChannel, channelName)
			}
			
			// Ensure no duplicate channel names across different sessions
			if channels[channelName] {
				t.Errorf("Duplicate channel name detected: %s", channelName)
			}
			channels[channelName] = true
		}
	}
	
	t.Logf("✅ Validated unique channel names for %d sessions across %d message types", 
		len(sessionIDs), 4)
}