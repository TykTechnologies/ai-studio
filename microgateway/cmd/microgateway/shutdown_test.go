package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The server (which drains requests and flushes the analytics pulse) must be
// stopped while the edge client is still connected to control, and the
// analytics writer last.
func TestRunShutdown_StopsServerBeforeEdgeClient(t *testing.T) {
	var order []string
	step := func(name string) func() { return func() { order = append(order, name) } }

	runShutdown(context.Background(), shutdownSteps{
		stopServer:        func(context.Context) error { order = append(order, "server"); return nil },
		stopBackground:    step("background"),
		stopControlServer: step("control-server"),
		stopEdgeClient:    step("edge-client"),
		stopHubSpoke:      step("hub-spoke"),
		cleanup:           step("cleanup"),
	})

	assert.Equal(t, []string{"server", "background", "control-server", "edge-client", "hub-spoke", "cleanup"}, order)
}

func TestRunShutdown_SkipsMissingSteps(t *testing.T) {
	assert.NotPanics(t, func() { runShutdown(context.Background(), shutdownSteps{}) })
}
