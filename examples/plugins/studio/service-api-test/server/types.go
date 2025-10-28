package main

import (
	"fmt"
	"time"
)

// TestResult represents the outcome of a single API operation test
type TestResult struct {
	Operation string        `json:"operation"`
	Success   bool          `json:"success"`
	Message   string        `json:"message"`
	Duration  time.Duration `json:"duration_ms"`
	Timestamp time.Time     `json:"timestamp"`
}

// Helper to generate unique test names with timestamp
func generateTestName(prefix string) string {
	return fmt.Sprintf("[E2E-TEST] %s-%d", prefix, time.Now().Unix())
}
