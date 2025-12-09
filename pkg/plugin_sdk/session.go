// Package plugin_sdk provides session management for long-lived broker connections.
// The session pattern keeps go-plugin broker connections alive for plugins that need
// background services like event pub/sub.
package plugin_sdk

import (
	"sync"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
)

// Default session timeout in milliseconds
const DefaultSessionTimeoutMs = 30000

// SessionState manages the state of a plugin session.
// A session keeps the broker connection alive for background services.
type SessionState struct {
	mu          sync.RWMutex
	active      bool
	sessionID   string
	brokerID    uint32
	broker      *goplugin.GRPCBroker
	closeChan   chan struct{}
	closeReason string
	firstOpen   bool // true if this is the first OpenSession call
}

// Global session instance - one session per plugin process
var (
	globalSession     *SessionState
	globalSessionOnce sync.Once
)

// getGlobalSession returns the singleton session instance
func getGlobalSession() *SessionState {
	globalSessionOnce.Do(func() {
		globalSession = &SessionState{
			closeChan: make(chan struct{}),
			firstOpen: true,
		}
	})
	return globalSession
}

// InitSession initializes the session with broker access.
// Called when OpenSession RPC is received from the host.
// Returns true if this is the first session (services should be started).
func InitSession(sessionID string, brokerID uint32, broker *goplugin.GRPCBroker) (firstSession bool, err error) {
	s := getGlobalSession()
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this is the first open or a renewal
	firstSession = s.firstOpen
	s.firstOpen = false

	// Update session state
	s.sessionID = sessionID
	s.brokerID = brokerID
	s.broker = broker
	s.active = true
	s.closeReason = ""

	// Reset close channel if it was closed
	select {
	case <-s.closeChan:
		// Channel was closed, create new one
		s.closeChan = make(chan struct{})
	default:
		// Channel is still open
	}

	return firstSession, nil
}

// WaitForClose blocks until the session is closed or times out.
// Returns the reason for closing ("timeout", "explicit_close", etc.)
// Called from the OpenSession RPC handler.
func WaitForClose(timeoutMs int32) string {
	s := getGlobalSession()

	// Use default timeout if not specified
	if timeoutMs <= 0 {
		timeoutMs = DefaultSessionTimeoutMs
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond

	select {
	case <-s.closeChan:
		// Explicit close via CloseSession
		s.mu.RLock()
		reason := s.closeReason
		s.mu.RUnlock()
		if reason == "" {
			reason = "explicit_close"
		}
		return reason
	case <-time.After(timeout):
		// Session timed out - host should re-open
		return "timeout"
	}
}

// CloseSession closes the active session.
// Called from the CloseSession RPC handler.
func CloseSession(reason string) {
	s := getGlobalSession()
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return
	}

	s.closeReason = reason
	s.active = false

	// Signal WaitForClose to return
	select {
	case <-s.closeChan:
		// Already closed
	default:
		close(s.closeChan)
	}
}

// IsSessionActive returns true if there's an active session.
func IsSessionActive() bool {
	s := getGlobalSession()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// GetSessionBroker returns the broker from the active session.
// Returns nil if no session is active.
func GetSessionBroker() *goplugin.GRPCBroker {
	s := getGlobalSession()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.active {
		return nil
	}
	return s.broker
}

// GetSessionBrokerID returns the broker ID from the active session.
// Returns 0 if no session is active.
func GetSessionBrokerID() uint32 {
	s := getGlobalSession()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.active {
		return 0
	}
	return s.brokerID
}

// GetSessionID returns the current session ID.
// Returns empty string if no session is active.
func GetSessionID() string {
	s := getGlobalSession()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

// ResetSession resets the session state for testing purposes.
// Should not be called in production code.
func ResetSession() {
	s := getGlobalSession()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.active = false
	s.sessionID = ""
	s.brokerID = 0
	s.broker = nil
	s.closeReason = ""
	s.firstOpen = true

	// Reset close channel
	select {
	case <-s.closeChan:
		// Already closed
	default:
		close(s.closeChan)
	}
	s.closeChan = make(chan struct{})
}
