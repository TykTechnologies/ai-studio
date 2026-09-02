//go:build !enterprise

package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Filter scripts are an enterprise feature: in CE the script runner is a
// pass-through. These tests pin that down, so a blocking filter attached in a
// CE deployment is a documented no-op rather than a surprise, and so the
// enterprise block tests cannot be mistaken for coverage of this build.
func TestToolRequest_CEDoesNotRunFilters(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"GET success"}`)
	defer f.teardown()

	f.attachFilter(t, "deny-input", `output := {block: true, message: "blocked"}`, false)
	f.attachFilter(t, "deny-output", `output := {block: true, message: "blocked"}`, true)

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code, "CE has no script runtime, so nothing blocks")
	assert.Contains(t, rr.Body.String(), "GET success")
	require.Len(t, f.upstream.ReceivedRequests, 1)
}

func TestMCPToolCall_CEDoesNotRunFilters(t *testing.T) {
	f := newToolFilterFixture(t, `{"ssn":"123-45-6789"}`)
	defer f.teardown()

	f.attachFilter(t, "deny-output", `output := {block: true, message: "blocked"}`, true)

	session := newMCPSession(t, f)
	text, isError := session.callTool(t, "getTestData", map[string]interface{}{})

	assert.False(t, isError, "CE has no script runtime, so nothing blocks")
	assert.Contains(t, text, "123-45-6789")
	require.Len(t, f.upstream.ReceivedRequests, 1)
}

// Attaching filters must not break a CE deployment's tool traffic either.
func TestToolRequest_CEUnfilteredToolStillWorks(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"GET success"}`)
	defer f.teardown()

	rr := f.call(t, "getTestData", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "GET success")
}
