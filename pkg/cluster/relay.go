package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/simonfxr/pubsub"
)

// RelayTopic is the cluster log topic bus events travel on.
const RelayTopic = "bus.event"

// RelayedByDefault reports whether a bus event must reach every replica:
// events for edges (DirDown), which each replica forwards to the edges whose
// streams it holds, and object change events (system.*), which keep every
// replica's caches and plugins current.
func RelayedByDefault(ev eventbridge.Event) bool {
	return ev.Dir == eventbridge.DirDown || strings.HasPrefix(ev.Topic, "system.")
}

// RelayOptions tune a Relay. The zero value gives the defaults.
type RelayOptions struct {
	// Filter picks the events to relay (default RelayedByDefault).
	Filter func(eventbridge.Event) bool
	// QueueSize bounds the events waiting to be written to the log
	// (default 4096). When it is full, events are dropped and counted.
	QueueSize int
	// PublishTimeout bounds one write to the log (default 5 s); a failed
	// write is retried with backoff up to MaxRetries times (default 5).
	PublishTimeout time.Duration
	MaxRetries     int
}

func (o RelayOptions) withDefaults() RelayOptions {
	if o.Filter == nil {
		o.Filter = RelayedByDefault
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 4096
	}
	if o.PublishTimeout <= 0 {
		o.PublishTimeout = 5 * time.Second
	}
	if o.MaxRetries <= 0 {
		o.MaxRetries = 5
	}
	return o
}

// RelayStats describe a relay's traffic, for the cluster status page.
type RelayStats struct {
	Sent     uint64 // events written to the log
	Received uint64 // events from other replicas republished locally
	Dropped  uint64 // events lost: queue full, or the log write kept failing
}

// Relay carries bus events between control-plane replicas through the
// cluster event log. Events published on this replica's bus that the filter
// selects are written to the log; events other replicas wrote are published
// on this replica's bus with RelayedFrom set, which also stops them being
// relayed again. On a database without the log (SQLite, one process) it
// relays nothing.
//
// Delivery follows the log: at least once to every live replica, in order
// per publishing replica. Bus events keep their ID, so consumers that
// persist events (webhooks) store each one once.
type Relay struct {
	log  *Log
	bus  eventbridge.Bus
	opts RelayOptions

	queue    chan eventbridge.Event
	sub      *pubsub.Subscription
	unsubLog func()
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	// publishing is held while an event from the log is published on the
	// bus, and by Stop before it unsubscribes from the bus: the bus
	// library's Unsubscribe is not synchronised with a Publish in flight on
	// another goroutine. closed (under it) turns later events away.
	publishing sync.Mutex
	closed     bool

	sent, received, dropped atomic.Uint64
}

// NewRelay returns a relay between bus and log. It does nothing until Start.
func NewRelay(log *Log, bus eventbridge.Bus, opts RelayOptions) *Relay {
	opts = opts.withDefaults()
	return &Relay{
		log:   log,
		bus:   bus,
		opts:  opts,
		queue: make(chan eventbridge.Event, opts.QueueSize),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Start begins relaying in both directions.
func (r *Relay) Start() {
	if !r.log.Enabled() {
		close(r.done)
		return
	}
	r.unsubLog = r.log.Subscribe(RelayTopic, r.fromCluster)
	r.sub = r.bus.SubscribeAll(r.fromBus)
	go r.run()
}

// Stop ends relaying. Events still queued are written first, within the
// publish timeout each.
func (r *Relay) Stop() {
	r.stopOnce.Do(func() {
		// Stop taking the log's events, and wait for one being published,
		// before leaving the bus.
		if r.unsubLog != nil {
			r.unsubLog()
		}
		r.publishing.Lock()
		r.closed = true
		r.publishing.Unlock()
		if r.sub != nil {
			r.bus.Unsubscribe(r.sub)
		}
		close(r.stop)
		<-r.done
	})
}

// Stats returns the relay's counters.
func (r *Relay) Stats() RelayStats {
	return RelayStats{Sent: r.sent.Load(), Received: r.received.Load(), Dropped: r.dropped.Load()}
}

// fromBus runs on the publisher's goroutine (the bus is synchronous), so it
// only queues.
func (r *Relay) fromBus(ev eventbridge.Event) {
	if ev.RelayedFrom != "" || !r.opts.Filter(ev) {
		return
	}
	select {
	case r.queue <- ev:
	default:
		if n := r.dropped.Add(1); n == 1 || n%100 == 0 {
			logger.Errorf("Cluster relay: queue full, dropped %s (%d dropped so far); other replicas and their edges miss it", ev.Topic, n)
		}
	}
}

func (r *Relay) run() {
	defer close(r.done)
	// A panic loses the event being written, not the relay.
	safe.Loop("cluster relay", r.stop, r.loop)
}

func (r *Relay) loop() {
	for {
		select {
		case ev := <-r.queue:
			r.write(ev)
		case <-r.stop:
			for {
				select {
				case ev := <-r.queue:
					r.write(ev)
				default:
					return
				}
			}
		}
	}
}

func (r *Relay) write(ev eventbridge.Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		r.dropped.Add(1)
		logger.Errorf("Cluster relay: cannot encode %s: %v", ev.Topic, err)
		return
	}
	backoff := 100 * time.Millisecond
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), r.opts.PublishTimeout)
		err = r.log.Publish(ctx, RelayTopic, payload)
		cancel()
		if err == nil {
			r.sent.Add(1)
			return
		}
		if attempt >= r.opts.MaxRetries {
			r.dropped.Add(1)
			logger.Errorf("Cluster relay: could not write %s (event %s) to the cluster log after %d attempts; other replicas and their edges miss it: %v", ev.Topic, ev.ID, attempt, err)
			return
		}
		select {
		case <-time.After(backoff):
		case <-r.stop:
			// Still try the remaining attempts on the way out, without waiting.
		}
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}

// fromCluster runs on the log's reader goroutine: it must be quick, and
// the bus's subscribers run inside it.
func (r *Relay) fromCluster(e Event) {
	var ev eventbridge.Event
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		logger.Errorf("Cluster relay: event %d from %s cannot be decoded; skipped: %v", e.ID, e.Origin, err)
		return
	}
	ev.RelayedFrom = e.Origin
	r.publishing.Lock()
	defer r.publishing.Unlock()
	if r.closed {
		return
	}
	r.received.Add(1)
	r.bus.Publish(ev)
}
