package studio

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
)

// signalTopic is the cluster log topic replica signals (pkg/replicas)
// travel on; the payload is the signal's name.
const signalTopic = "replica.signal"

// replicaBackend connects pkg/replicas to this Studio's cluster membership.
type replicaBackend struct{ s *Studio }

func (b replicaBackend) IsLeader() bool { return b.s.leadership.IsLeader() }

func (b replicaBackend) Signal(ctx context.Context, name string) error {
	return b.s.clusterLog.Publish(ctx, signalTopic, []byte(name))
}

// connectReplicas makes pkg/replicas answer for this Studio: leadership
// from the leader lease, signals over the cluster event log.
func (s *Studio) connectReplicas() {
	s.unsubscribeSignals = s.clusterLog.Subscribe(signalTopic, func(ev cluster.Event) {
		replicas.Deliver(string(ev.Payload))
	})
	replicas.SetBackend(replicaBackend{s})
}

// coalescer runs fn in the background after a trigger, once for any number
// of triggers that arrive while it waits or runs. Changes relayed from
// other replicas arrive on the cluster log's reader goroutine, which must
// not block, and often in bursts (a bulk import).
type coalescer struct {
	name  string
	delay time.Duration
	fn    func() error

	mu      sync.Mutex
	pending bool
	running bool
	stopped bool
	wg      sync.WaitGroup
}

func newCoalescer(name string, delay time.Duration, fn func() error) *coalescer {
	return &coalescer{name: name, delay: delay, fn: fn}
}

func (c *coalescer) trigger() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.pending = true
	if c.running {
		return
	}
	c.running = true
	c.wg.Add(1)
	go c.loop()
}

func (c *coalescer) loop() {
	defer c.wg.Done()
	for {
		time.Sleep(c.delay)
		c.mu.Lock()
		if !c.pending || c.stopped {
			c.running = false
			c.mu.Unlock()
			return
		}
		c.pending = false
		c.mu.Unlock()
		if err := c.fn(); err != nil {
			logger.Errorf("Applying a change from another replica (%s) failed; this replica may serve the old configuration until the next change: %v", c.name, err)
		}
	}
}

// stop waits for a run in progress and drops pending triggers.
func (c *coalescer) stop() {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
	c.wg.Wait()
}

// relayedChanges keeps this replica's in-memory state in step with the
// database when other replicas (or code paths that do not refresh it
// themselves) change it.
type relayedChanges struct {
	reloadGateway *coalescer
	clearBudgets  *coalescer
	// plugins applies other replicas' plugin changes, one at a time in
	// event order (starting a plugin takes seconds).
	plugins     chan pluginChange
	pluginsDone chan struct{}
	pluginsMu   sync.Mutex // guards sends against the close in stop
	closed      bool
	unsubscribe func()
}

type pluginChange struct {
	topic string
	id    uint
}

func (s *Studio) watchRelayedChanges(bus eventbridge.Bus) {
	service := s.service
	rc := &relayedChanges{
		// The embedded gateway serves LLMs (with their filters and
		// plugins) and datasources from memory.
		reloadGateway: newCoalescer("embedded gateway reload", 200*time.Millisecond, func() error {
			if s.proxy == nil {
				return nil
			}
			return s.proxy.Reload()
		}),
		// Budget checks cache spend per App and team.
		clearBudgets: newCoalescer("budget cache", 50*time.Millisecond, func() error {
			if service.Budget != nil {
				service.Budget.ClearCache()
			}
			if service.TeamBudget != nil {
				service.TeamBudget.ClearCache()
			}
			return nil
		}),
	}
	rc.plugins = make(chan pluginChange, 1024)
	rc.pluginsDone = make(chan struct{})
	go func() {
		defer close(rc.pluginsDone)
		for ch := range rc.plugins {
			service.ApplyPluginChangeFromReplica(ch.topic, ch.id)
		}
	}()
	sub := bus.SubscribeAll(func(ev eventbridge.Event) {
		if ev.RelayedFrom != "" && strings.HasPrefix(ev.Topic, "system.plugin.") {
			var p struct {
				ObjectID uint `json:"object_id"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err == nil && p.ObjectID != 0 {
				rc.pluginsMu.Lock()
				if !rc.closed {
					select {
					case rc.plugins <- pluginChange{topic: ev.Topic, id: p.ObjectID}:
					default:
						logger.Errorf("Too many plugin changes from other replicas waiting; plugin %d keeps its current state here until it changes again or this replica restarts", p.ObjectID)
					}
				}
				rc.pluginsMu.Unlock()
			}
		}
		switch {
		// Local or relayed: not every write path here reloads the gateway
		// itself (deleting an LLM, any datasource or filter change).
		case strings.HasPrefix(ev.Topic, "system.llm."), strings.HasPrefix(ev.Topic, "system.datasource."),
			strings.HasPrefix(ev.Topic, "system.filter."), strings.HasPrefix(ev.Topic, "system.plugin."):
			rc.reloadGateway.trigger()
		case ev.RelayedFrom != "" && strings.HasPrefix(ev.Topic, "system.app."):
			rc.clearBudgets.trigger()
		}
	})
	removeSignal := replicas.OnSignal(replicas.SignalBudgets, rc.clearBudgets.trigger)
	rc.unsubscribe = func() {
		bus.Unsubscribe(sub)
		removeSignal()
	}
	s.relayed = rc
}

func (rc *relayedChanges) stop() {
	if rc == nil {
		return
	}
	rc.unsubscribe()
	rc.pluginsMu.Lock()
	rc.closed = true
	close(rc.plugins)
	rc.pluginsMu.Unlock()
	<-rc.pluginsDone
	rc.reloadGateway.stop()
	rc.clearBudgets.stop()
}
