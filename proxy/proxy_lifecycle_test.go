package proxy

import (
	"context"
	"testing"
)

// Stop may run before Start has created its server (an embedding host
// shutting down early); it must neither panic nor leave Start to listen.
func TestStopBeforeStartIsSafe(t *testing.T) {
	p := &Proxy{config: &Config{}}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if !p.stopped {
		t.Error("Stop should mark the proxy stopped so a later Start does not listen")
	}
}
