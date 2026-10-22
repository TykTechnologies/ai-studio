package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	eventpb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// callBrokerGrace is how long a per-call brokered server stays up after its
// call returns, waiting for a dial that may still be on its way. go-plugin
// keeps the connection info for five seconds after the host accepts, so a
// plugin cannot connect later than that anyway.
var callBrokerGrace = 10 * time.Second

// trackedBroker is a brokered service server and whether the plugin has
// connected to it. It is the server's stats handler.
type trackedBroker struct {
	connected atomic.Bool
	// anyConnected is the plugin client's hostConnected, set with connected.
	anyConnected *atomic.Bool
	mu           sync.Mutex
	server       *grpc.Server
	stopped      bool
}

func (b *trackedBroker) HandleConn(_ context.Context, s stats.ConnStats) {
	if _, ok := s.(*stats.ConnBegin); ok {
		b.connected.Store(true)
		if b.anyConnected != nil {
			b.anyConnected.Store(true)
		}
	}
}

func (b *trackedBroker) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (b *trackedBroker) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context { return ctx }
func (b *trackedBroker) HandleRPC(context.Context, stats.RPCStats)                       {}

// setServer records the server go-plugin built, stopping it at once if
// stop already ran.
func (b *trackedBroker) setServer(s *grpc.Server) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.server = s
	if b.stopped {
		s.Stop()
	}
}

func (b *trackedBroker) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = true
	if b.server != nil {
		b.server.Stop()
	}
}

// startTrackedBroker serves the AI Studio management and event services to
// plugin pluginID on a fresh broker ID, and returns the ID once the server
// is up (or waitBrokeredServer gave up on it).
func (c *AIStudioPluginClient) startTrackedBroker(pluginID uint, purpose string) (uint32, *trackedBroker) {
	brokerID := c.broker.NextId()
	tracked := &trackedBroker{anyConnected: &c.hostConnected}
	service, evtServer := c.service, c.eventServer
	ready := make(chan struct{})
	go c.broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
		// The plugin's calls carry its ID and are checked against its
		// approved scopes.
		opts = append(PluginBrokerServerOptions(opts, pluginID, service), grpc.StatsHandler(tracked))
		s := grpc.NewServer(opts...)
		if NewAIStudioManagementServerFunc != nil {
			if impl, ok := NewAIStudioManagementServerFunc(service).(mgmtpb.AIStudioManagementServiceServer); ok {
				mgmtpb.RegisterAIStudioManagementServiceServer(s, impl)
			}
		}
		if evtServer != nil {
			eventpb.RegisterPluginEventServiceServer(s, evtServer)
		}
		tracked.setServer(s)
		close(ready)
		return s
	})
	waitBrokeredServer(ready, brokerID, pluginID, purpose)
	return brokerID, tracked
}

// startCallBroker serves the host services on a fresh broker ID for one
// call (a scheduled task, an edge payload, an agent session). The fresh ID
// is what lets a plugin built before AI Studio 2.3, which dials lazily,
// reach host services from inside the call: go-plugin hands over its
// connection info at call time.
//
// A plugin can only connect within go-plugin's five seconds of the accept,
// so a server it has not connected to callBrokerGrace later never will be:
// it is stopped then, whether or not the call is still running. One the
// plugin connected to stays, since the plugin keeps using that connection.
// Each call used to leave its server (a goroutine and a listener) running
// for good.
func (c *AIStudioPluginClient) startCallBroker(pluginID uint, purpose string) uint32 {
	brokerID, tracked := c.startTrackedBroker(pluginID, purpose)
	time.AfterFunc(callBrokerGrace, func() {
		if tracked.connected.Load() {
			return
		}
		tracked.stop()
		log.Debug().Uint32("broker_id", brokerID).Uint("plugin_id", pluginID).Str("purpose", purpose).
			Msg("Stopped unused per-call brokered server")
	})
	return brokerID
}
