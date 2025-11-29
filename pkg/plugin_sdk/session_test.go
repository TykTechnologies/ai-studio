package plugin_sdk

import (
	"sync"
	"testing"
	"time"
)

func TestSessionState_InitSession(t *testing.T) {
	// Reset global state before test
	ResetSession()

	// First init should return firstSession=true
	firstSession, err := InitSession("test-session-1", 123, nil)
	if err != nil {
		t.Fatalf("InitSession failed: %v", err)
	}
	if !firstSession {
		t.Error("First InitSession should return firstSession=true")
	}

	// Check session is active
	if !IsSessionActive() {
		t.Error("Session should be active after InitSession")
	}

	// Check broker ID is stored
	if GetSessionBrokerID() != 123 {
		t.Errorf("Expected broker ID 123, got %d", GetSessionBrokerID())
	}

	// Second init (simulating session renewal after timeout) should return firstSession=false
	// First close the current session
	CloseSession("test-close")

	// Wait for close to complete
	time.Sleep(10 * time.Millisecond)

	// Re-init with same session ID should return firstSession=false
	firstSession, err = InitSession("test-session-1", 124, nil)
	if err != nil {
		t.Fatalf("Second InitSession failed: %v", err)
	}
	if firstSession {
		t.Error("Second InitSession should return firstSession=false (session renewal)")
	}
}

func TestSessionState_WaitForClose_Timeout(t *testing.T) {
	// Reset global state before test
	ResetSession()

	// Init session
	_, err := InitSession("test-session-timeout", 100, nil)
	if err != nil {
		t.Fatalf("InitSession failed: %v", err)
	}

	// Wait for close with short timeout
	start := time.Now()
	reason := WaitForClose(100) // 100ms timeout
	elapsed := time.Since(start)

	// Should return "timeout"
	if reason != "timeout" {
		t.Errorf("Expected reason 'timeout', got '%s'", reason)
	}

	// Should have taken approximately 100ms
	if elapsed < 80*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("WaitForClose took %v, expected ~100ms", elapsed)
	}
}

func TestSessionState_WaitForClose_ExplicitClose(t *testing.T) {
	// Reset global state before test
	ResetSession()

	// Init session
	_, err := InitSession("test-session-explicit", 100, nil)
	if err != nil {
		t.Fatalf("InitSession failed: %v", err)
	}

	// Close session from another goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		CloseSession("explicit_close_test")
	}()

	// Wait for close
	start := time.Now()
	reason := WaitForClose(5000) // 5 second timeout (should not reach)
	elapsed := time.Since(start)

	// Should return our close reason
	if reason != "explicit_close_test" {
		t.Errorf("Expected reason 'explicit_close_test', got '%s'", reason)
	}

	// Should have taken approximately 50ms (not 5 seconds)
	if elapsed < 40*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("WaitForClose took %v, expected ~50ms", elapsed)
	}

	wg.Wait()
}

func TestSessionState_ConcurrentAccess(t *testing.T) {
	// Reset global state before test
	ResetSession()

	// Init session
	_, err := InitSession("test-session-concurrent", 100, nil)
	if err != nil {
		t.Fatalf("InitSession failed: %v", err)
	}

	// Concurrent access to session state
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = IsSessionActive()
			_ = GetSessionBrokerID()
			_ = GetSessionBroker()
		}()
	}
	wg.Wait()

	// Close from one goroutine while others are reading
	go func() {
		time.Sleep(10 * time.Millisecond)
		CloseSession("concurrent_test")
	}()

	// Should complete without race condition
	_ = WaitForClose(1000)
}

func TestSessionState_GetSessionID(t *testing.T) {
	// Reset global state before test
	ResetSession()

	// Initially, session ID should be empty
	if GetSessionID() != "" {
		t.Error("Session ID should be empty before InitSession")
	}

	// After init, session ID should be set
	_, err := InitSession("my-test-session", 100, nil)
	if err != nil {
		t.Fatalf("InitSession failed: %v", err)
	}

	if GetSessionID() != "my-test-session" {
		t.Errorf("Expected session ID 'my-test-session', got '%s'", GetSessionID())
	}
}
