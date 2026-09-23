package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestBudgetDenial(t *testing.T) {
	status, msg := budgetDenial(errors.New("budget exceeded: current=10"), "Budget limit exceeded")
	if status != http.StatusForbidden || msg != "Budget limit exceeded" {
		t.Fatalf("a budget decision must stay a 403: %d %q", status, msg)
	}

	unavailable := fmt.Errorf("%w: reading app 1: database is locked", services.ErrBudgetCheckUnavailable)
	status, msg = budgetDenial(unavailable, "Budget limit exceeded")
	if status != http.StatusServiceUnavailable || msg == "Budget limit exceeded" {
		t.Fatalf("a check that could not run must be a 503, not a spent budget: %d %q", status, msg)
	}
}

// The default transport keeps 2 idle connections per host; with more requests
// in flight to one vendor that forces a new TCP+TLS handshake for most of them.
func TestUpstreamAndLoopbackPoolsKeepConnections(t *testing.T) {
	up := upstreamGuardedTransport
	if up.MaxIdleConnsPerHost < 64 || up.MaxIdleConns < up.MaxIdleConnsPerHost {
		t.Fatalf("upstream pool too small: per-host %d, total %d", up.MaxIdleConnsPerHost, up.MaxIdleConns)
	}
	lb := newLoopbackTransport(false)
	if lb.MaxIdleConnsPerHost < 64 {
		t.Fatalf("loopback pool too small: per-host %d", lb.MaxIdleConnsPerHost)
	}
}
