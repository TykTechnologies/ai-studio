package chat_session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestQueueDuplicationFix tests the queue behavior to ensure no duplicates
func TestQueueDuplicationFix(t *testing.T) {
	t.Run("InMemory queue - message vs stream separation", func(t *testing.T) {
		queue := NewInMemoryQueue("test-session", 10)
		defer queue.Close()

		ctx := context.Background()

		// Simulate what the fix does: only send final message to PublishMessage
		finalContent := "This is the final response"
		err := queue.PublishMessage(ctx, &ChatResponse{Payload: finalContent})
		assert.NoError(t, err)

		// DO NOT send to PublishStream (this was the cause of duplicates)
		// err = queue.PublishStream(ctx, []byte(finalContent)) // REMOVED

		time.Sleep(10 * time.Millisecond)

		// Count messages in both channels
		messageCount := 0
		streamCount := 0

		// Check OutputMessage channel
		for {
			select {
			case msg := <-queue.ConsumeMessages(ctx):
				messageCount++
				assert.Equal(t, finalContent, msg.Payload)
			case <-time.After(10 * time.Millisecond):
				goto messagesDone
			}
		}

	messagesDone:
		// Check OutputStream channel
		for {
			select {
			case stream := <-queue.ConsumeStream(ctx):
				streamCount++
				if string(stream) == finalContent {
					t.Errorf("Final message should not appear in stream channel")
				}
			case <-time.After(10 * time.Millisecond):
				goto streamDone
			}
		}

	streamDone:
		assert.Equal(t, 1, messageCount, "Should have exactly one message")
		assert.Equal(t, 0, streamCount, "Should have no duplicate in stream")
	})

	t.Run("Status message deduplication", func(t *testing.T) {
		queue := NewInMemoryQueue("test-session", 10)
		defer queue.Close()

		ctx := context.Background()

		// Simulate what sendStatus does after the fix
		statusMsg := ":::system Tool added:::"
		err := queue.PublishMessage(ctx, &ChatResponse{Payload: statusMsg})
		assert.NoError(t, err)

		// DO NOT send to PublishStream (this was causing duplicates)
		// err = queue.PublishStream(ctx, []byte(statusMsg)) // REMOVED

		time.Sleep(10 * time.Millisecond)

		messageCount := 0
		streamCount := 0

		// Drain messages
		for {
			select {
			case msg := <-queue.ConsumeMessages(ctx):
				messageCount++
				assert.Contains(t, msg.Payload, ":::system")
			case <-time.After(10 * time.Millisecond):
				goto statusMessagesDone
			}
		}

	statusMessagesDone:
		// Check stream channel
		for {
			select {
			case stream := <-queue.ConsumeStream(ctx):
				streamCount++
				if strings.Contains(string(stream), "Tool added") {
					t.Errorf("Status message should not appear in stream channel")
				}
			case <-time.After(10 * time.Millisecond):
				goto statusStreamDone
			}
		}

	statusStreamDone:
		assert.Equal(t, 1, messageCount, "Should have exactly one status message")
		assert.Equal(t, 0, streamCount, "Should have no status duplicate in stream")
	})

	t.Run("Streaming chunks vs final message", func(t *testing.T) {
		queue := NewInMemoryQueue("test-session", 10)
		defer queue.Close()

		ctx := context.Background()

		// Simulate streaming chunks during actual streaming
		err := queue.PublishStream(ctx, []byte("Hello "))
		assert.NoError(t, err)
		err = queue.PublishStream(ctx, []byte("world"))
		assert.NoError(t, err)

		// Simulate final complete message (after fix - only to PublishMessage)
		finalMsg := "Hello world!"
		err = queue.PublishMessage(ctx, &ChatResponse{Payload: finalMsg})
		assert.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		// Collect all stream chunks
		var streamChunks []string
		for {
			select {
			case stream := <-queue.ConsumeStream(ctx):
				streamChunks = append(streamChunks, string(stream))
			case <-time.After(10 * time.Millisecond):
				goto streamCollectDone
			}
		}

	streamCollectDone:
		// Collect final message
		var finalMessages []string
		for {
			select {
			case msg := <-queue.ConsumeMessages(ctx):
				finalMessages = append(finalMessages, msg.Payload)
			case <-time.After(10 * time.Millisecond):
				goto finalCollectDone
			}
		}

	finalCollectDone:
		// Verify streaming chunks
		assert.Len(t, streamChunks, 2, "Should have received 2 stream chunks")
		assert.Contains(t, streamChunks, "Hello ")
		assert.Contains(t, streamChunks, "world")

		// Verify final message
		assert.Len(t, finalMessages, 1, "Should have exactly one final message")
		assert.Equal(t, finalMsg, finalMessages[0])

		// Most importantly: final complete message should NOT be in stream chunks
		assert.NotContains(t, streamChunks, finalMsg, "Complete final message should not be in stream chunks")
	})
}

// TestDuplicateBehaviorBefore simulates the old buggy behavior for comparison
func TestDuplicateBehaviorBefore(t *testing.T) {
	t.Run("Old behavior would cause duplicates", func(t *testing.T) {
		queue := NewInMemoryQueue("test-session", 10)
		defer queue.Close()

		ctx := context.Background()

		// Simulate the OLD buggy behavior (sending to both channels)
		finalContent := "This would be duplicated"
		
		// This is what the code used to do (causing duplicates):
		err := queue.PublishMessage(ctx, &ChatResponse{Payload: finalContent})
		assert.NoError(t, err)
		err = queue.PublishStream(ctx, []byte(finalContent)) // This caused the duplicate!
		assert.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		messageCount := 0
		streamCount := 0
		var messageContent, streamContent string

		// Count messages in both channels
		for {
			select {
			case msg := <-queue.ConsumeMessages(ctx):
				messageCount++
				messageContent = msg.Payload
			case stream := <-queue.ConsumeStream(ctx):
				streamCount++
				streamContent = string(stream)
			case <-time.After(10 * time.Millisecond):
				goto oldBehaviorDone
			}
		}

	oldBehaviorDone:
		// This demonstrates the problem that existed before the fix
		assert.Equal(t, 1, messageCount, "One message in OutputMessage")
		assert.Equal(t, 1, streamCount, "One message in OutputStream")
		assert.Equal(t, finalContent, messageContent)
		assert.Equal(t, finalContent, streamContent)
		
		// This shows why we had duplicates - same content in both channels
		assert.Equal(t, messageContent, streamContent, "Same content in both channels = duplicate in UI")
		
		t.Logf("OLD BEHAVIOR: Message='%s', Stream='%s' (DUPLICATE!)", messageContent, streamContent)
	})
}