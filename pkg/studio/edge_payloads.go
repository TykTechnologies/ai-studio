package studio

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Edge-to-control traffic on a headless control plane. Its edges send two
// kinds of it, and both are for plugins, which a headless replica does not
// run:
//
//   - events edges publish with DirUp: the headless relay also relays those
//     (headlessRelayFilter), so every full replica's plugins get them, as
//     they get the events of their own edges;
//   - plugin payloads (SendPluginControlBatch): each goes on the cluster log
//     under pluginControlTopic (edgePayloadForwarder), and the leader, which
//     is always a full Studio since a headless replica never leads, hands it
//     to its plugin (edgePayloadHost).

// pluginControlTopic is the cluster log topic edges' plugin payloads travel
// on, from a headless replica to the leader. One row per payload (edges cap a
// payload at 1 MB), its proto encoding.
const pluginControlTopic = "plugin.control"

// headlessRelayFilter picks the bus events a headless replica relays: those
// every replica relays, and whatever its edges sent (FromEdge), so that the
// plugins on the full replicas see them. A full replica keeps
// cluster.RelayedByDefault: its own plugins already had its edges' events.
// An edge's system.* events are not relayed: object changes come from
// Studio.
func headlessRelayFilter(ev eventbridge.Event) bool {
	if ev.FromEdge {
		return !strings.HasPrefix(ev.Topic, "system.")
	}
	return cluster.RelayedByDefault(ev)
}

func encodeEdgePayload(p *pb.PluginControlPayload) ([]byte, error) {
	return proto.Marshal(p)
}

func decodeEdgePayload(b []byte) (*pb.PluginControlPayload, error) {
	p := &pb.PluginControlPayload{}
	if err := proto.Unmarshal(b, p); err != nil {
		return nil, err
	}
	return p, nil
}

// edgePayloadForwarder puts a headless replica's edge plugin payloads on the
// cluster log for the leader (grpc.EdgePayloadForwarder).
type edgePayloadForwarder struct{ log *cluster.Log }

func (f edgePayloadForwarder) ForwardEdgePayloads(ctx context.Context, payloads []*pb.PluginControlPayload) error {
	if !f.log.Enabled() {
		return errors.New("no cluster log to reach the plugin host")
	}
	rows := make([][]byte, len(payloads))
	for i, p := range payloads {
		b, err := encodeEdgePayload(p)
		if err != nil {
			return err
		}
		rows[i] = b
	}
	// One row per payload, one write for the batch.
	return f.log.PublishBatch(ctx, pluginControlTopic, rows)
}

const (
	// edgePayloadQueue bounds the payloads waiting for a plugin; more are
	// dropped (and counted).
	edgePayloadQueue = 1024
	// edgePayloadCatchUpPage is how many rows catching up reads at a time.
	edgePayloadCatchUpPage = 256
	// edgePayloadTimeout bounds one plugin's handling of a payload.
	edgePayloadTimeout = 30 * time.Second
	// edgePayloadReplay is how far back a replica that has just become the
	// leader reads the log for payloads: the leader lease's TTL (Studio uses
	// the default) and a margin, which covers what arrived while no replica
	// led (after the previous leader crashed) and what the previous leader
	// may not have handled. Payloads the previous leader did handle in that
	// window are handed to plugins again.
	edgePayloadReplay = cluster.DefaultLeaseTTL + 15*time.Second
	// edgePayloadSeen is how long a handled row's id is remembered: the
	// log's retention (Studio uses the default), beyond which the log
	// delivers nothing again.
	edgePayloadSeen = cluster.DefaultLogRetention
	// edgePayloadPrune is how often ids older than edgePayloadSeen are
	// forgotten.
	edgePayloadPrune = time.Minute
)

type queuedEdgePayload struct {
	id      int64
	payload *pb.PluginControlPayload
}

// edgePayloadHost hands edge plugin payloads that headless replicas forward
// to this replica's plugins, while this replica leads: one replica (the
// leader) handles each payload. Delivery follows the log, at least once, and
// a new leader handles again what arrived in the edgePayloadReplay window
// before it took over. Plugins tell repeats apart by correlation ID.
type edgePayloadHost struct {
	db     *gorm.DB
	log    *cluster.Log
	router atomic.Value // grpc.EdgePayloadRouter

	queue chan queuedEdgePayload
	ctx   context.Context
	stopC context.CancelFunc
	wg    sync.WaitGroup

	mu      sync.Mutex
	seen    map[int64]time.Time
	stopped bool

	unsubscribe func()
	stopLeading func()
	routed      atomic.Uint64
	failed      atomic.Uint64
	dropped     atomic.Uint64
}

// newEdgePayloadHost starts handing payloads to router. log may be nil (in
// tests: payloads then come only through deliver).
func newEdgePayloadHost(db *gorm.DB, log *cluster.Log, router grpc.EdgePayloadRouter) *edgePayloadHost {
	ctx, cancel := context.WithCancel(context.Background())
	h := &edgePayloadHost{
		db:    db,
		log:   log,
		queue: make(chan queuedEdgePayload, edgePayloadQueue),
		ctx:   ctx,
		stopC: cancel,
		seen:  map[int64]time.Time{},
	}
	h.router.Store(routerBox{router})
	h.wg.Add(1)
	go h.run()
	if log != nil && log.Enabled() {
		h.unsubscribe = log.Subscribe(pluginControlTopic, h.deliver)
		h.stopLeading = replicas.OnLeading(h.catchUp)
		if replicas.IsLeader() {
			// Took the lease before this host listened for it.
			h.catchUp()
		}
	}
	return h
}

// routerBox lets atomic.Value hold an interface of varying dynamic types.
type routerBox struct{ grpc.EdgePayloadRouter }

// setRouter replaces the plugin router (tests).
func (h *edgePayloadHost) setRouter(r grpc.EdgePayloadRouter) { h.router.Store(routerBox{r}) }

// deliver takes a payload from the log. It runs on the log's reader
// goroutine, so it only queues.
func (h *edgePayloadHost) deliver(ev cluster.Event) {
	if !replicas.IsLeader() {
		// The leader handles it. If there is none right now, the next one
		// catches up on it (catchUp).
		return
	}
	h.accept(ev.ID, ev.Payload)
}

// accept queues a log row's payload once.
func (h *edgePayloadHost) accept(id int64, raw []byte) {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	if _, done := h.seen[id]; done {
		h.mu.Unlock()
		return
	}
	h.seen[id] = time.Now()
	h.mu.Unlock()

	p, err := decodeEdgePayload(raw)
	if err != nil {
		h.failed.Add(1)
		logger.Errorf("Edge plugin payload %d from the cluster log cannot be decoded; skipped: %v", id, err)
		return
	}
	select {
	case h.queue <- queuedEdgePayload{id: id, payload: p}:
	default:
		if n := h.dropped.Add(1); n == 1 || n%100 == 0 {
			logger.Errorf("Edge plugin payloads: queue full, dropped a payload for plugin %d (%d dropped so far)", p.PluginId, n)
		}
	}
}

// catchUp runs when this replica becomes the leader: it takes the payloads
// the log carried in the last edgePayloadReplay, which a leader that crashed
// may not have handled and which no replica took while none led.
func (h *edgePayloadHost) catchUp() {
	// Under mu: stop waits for the group only once stopped is set, so no
	// Add follows its Wait.
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.wg.Add(1)
	h.mu.Unlock()
	go func() {
		defer h.wg.Done()
		// A page at a time: every row in the window is read (one the queue
		// has no room for is dropped and counted in accept).
		var after int64
		for {
			var rows []models.ClusterEvent
			err := h.db.WithContext(h.ctx).
				Where("topic = ? AND created_at > now() - (? * interval '1 second') AND id > ?",
					pluginControlTopic, int(edgePayloadReplay.Seconds()), after).
				Order("id").Limit(edgePayloadCatchUpPage).Find(&rows).Error
			if err != nil {
				if h.ctx.Err() == nil {
					logger.Errorf("Edge plugin payloads: could not read the recent ones on becoming the leader; payloads sent while no replica led are lost: %v", err)
				}
				return
			}
			for _, r := range rows {
				if !replicas.IsLeader() {
					return
				}
				h.accept(r.ID, r.Payload)
				after = r.ID
			}
			if len(rows) < edgePayloadCatchUpPage {
				return
			}
		}
	}()
}

func (h *edgePayloadHost) run() {
	defer h.wg.Done()
	prune := time.NewTicker(edgePayloadPrune)
	defer prune.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case q := <-h.queue:
			h.route(q)
		case <-prune.C:
			h.forgetOld(time.Now().Add(-edgePayloadSeen))
		}
	}
}

// forgetOld drops the ids of rows handled before cutoff: the log no longer
// holds them, so they cannot arrive again.
func (h *edgePayloadHost) forgetOld(cutoff time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, at := range h.seen {
		if at.Before(cutoff) {
			delete(h.seen, id)
		}
	}
}

func (h *edgePayloadHost) route(q queuedEdgePayload) {
	defer func() {
		if r := recover(); r != nil {
			h.failed.Add(1)
			logger.Errorf("Edge plugin payload for plugin %d panicked in its plugin: %v", q.payload.PluginId, r)
		}
	}()
	ctx, cancel := context.WithTimeout(h.ctx, edgePayloadTimeout)
	defer cancel()
	router := h.router.Load().(routerBox)
	if err := router.RouteEdgePayload(ctx, q.payload); err != nil {
		if n := h.failed.Add(1); n == 1 || n%100 == 0 {
			logger.Warnf("Edge plugin payload for plugin %d (correlation %q, edge %s) was not handled (%d so far): %v",
				q.payload.PluginId, q.payload.CorrelationId, q.payload.EdgeId, n, err)
		}
		return
	}
	h.routed.Add(1)
}

// stop stops taking payloads and waits for the one being handled.
func (h *edgePayloadHost) stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.stopped = true
	h.mu.Unlock()
	if h.unsubscribe != nil {
		h.unsubscribe()
	}
	if h.stopLeading != nil {
		h.stopLeading()
	}
	h.stopC()
	h.wg.Wait()
}
