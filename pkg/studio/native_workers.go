package studio

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/proxy"
)

// startNativeWorkers starts the gateway's native resource reconciler
// (proxy.StartNativeWorkers); tests replace it.
var startNativeWorkers = func(p *proxy.Proxy, ctx context.Context, gate func() bool) (stop func()) {
	return p.StartNativeWorkers(ctx, gate)
}
