# Event Bridge Package

The `eventbridge` package provides an event bus system for communication between AI Studio (control) and Microgateway (edge) nodes. It uses `github.com/simonfxr/pubsub` for in-process event buses and integrates with the existing gRPC bidirectional streaming infrastructure for cross-node event forwarding.

## Architecture

```
┌──────────────────────────────────────┐     ┌──────────────────────────────────────┐
│         AI Studio (Control)          │     │        Microgateway (Edge)           │
│                                      │     │                                      │
│  ┌─────────────┐   ┌──────────┐     │     │     ┌──────────┐   ┌─────────────┐  │
│  │ Application │──▶│ EventBus │     │     │     │ EventBus │◀──│ Application │  │
│  └─────────────┘   └────┬─────┘     │     │     └────┬─────┘   └─────────────┘  │
│                         │           │     │          │                          │
│                    ┌────▼─────┐     │     │     ┌────▼─────┐                    │
│                    │  Bridge  │     │     │     │  Bridge  │                    │
│                    └────┬─────┘     │     │     └────┬─────┘                    │
│                         │           │     │          │                          │
│                    ┌────▼─────┐     │     │     ┌────▼─────┐                    │
│                    │  Stream  │◀───────────────▶│  Stream  │                    │
│                    │ Adapter  │     gRPC       │ Adapter  │                    │
│                    └──────────┘     │     │     └──────────┘                    │
└──────────────────────────────────────┘     └──────────────────────────────────────┘
```

## Event Direction

Events are routed based on their `Direction`:

- **DirLocal (0)**: Events stay on the local bus, never forwarded to remote nodes
- **DirUp (1)**: Events flow from edge to control
- **DirDown (2)**: Events flow from control to edge(s)

## Feedback Loop Prevention

The bridge prevents feedback loops through two mechanisms:

1. **Remote → Local conversion**: Events received from remote are always republished locally with `DirLocal`, preventing re-forwarding
2. **Role-based filtering**: Control only forwards `DirDown` events; edges only forward `DirUp` events

## Usage

### Publishing Events

```go
// Get the event bus from control server or edge client
bus := controlServer.GetEventBus() // or edgeClient.GetEventBus()

// Publish a local-only event
eventbridge.PublishLocal(bus, "node-id", "internal.log", map[string]string{"msg": "test"})

// Publish an event from edge to control
eventbridge.PublishUp(bus, "edge-123", "metrics.report", MetricsData{CPU: 50})

// Publish an event from control to edges
eventbridge.PublishDown(bus, "control", "config.reload", ConfigUpdate{Version: "1.0"})
```

### Subscribing to Events

```go
bus := controlServer.GetEventBus()

// Subscribe to specific topic
sub := bus.Subscribe("metrics.report", func(ev eventbridge.Event) {
    var data MetricsData
    json.Unmarshal(ev.Payload, &data)
    fmt.Printf("Received metrics from %s: CPU=%d\n", ev.Origin, data.CPU)
})
defer bus.Unsubscribe(sub)

// Subscribe to all events
allSub := bus.SubscribeAll(func(ev eventbridge.Event) {
    fmt.Printf("Event: topic=%s origin=%s\n", ev.Topic, ev.Origin)
})
defer bus.Unsubscribe(allSub)
```

## Components

### Event

The canonical event type with routing metadata:

```go
type Event struct {
    ID      string          // UUID for dedup/tracing
    Topic   string          // Logical topic name
    Origin  string          // Node ID that created the event
    Dir     Direction       // Routing direction
    Payload json.RawMessage // Application payload
}
```

### Bus

Interface for the local event bus:

```go
type Bus interface {
    Subscribe(topic string, fn func(Event)) *pubsub.Subscription
    SubscribeAll(fn func(Event)) *pubsub.Subscription
    Unsubscribe(sub *pubsub.Subscription)
    Publish(event Event)
}
```

### Bridge

Connects a local bus to a remote node via gRPC:

```go
bridge := eventbridge.NewBridge(eventbridge.BridgeConfig{
    NodeID:    "edge-001",
    IsControl: false,
    Topics:    []string{"metrics.*", "errors.*"}, // Optional topic filter
}, bus, streamAdapter)
bridge.Start(ctx)
defer bridge.Stop()
```

### StreamAdapter

Adapts gRPC streams to the Bridge interface:

```go
adapter := eventbridge.NewStreamAdapter(func(frame *eventbridge.EventFrame) error {
    return stream.Send(&pb.ControlMessage{
        Message: &pb.ControlMessage_Event{Event: protoFrame},
    })
}, 100) // buffer size
```

## Integration Points

### AI Studio (Control Server)

The `ControlServer` in `grpc/control_server.go`:
- Creates an event bus on startup
- Creates a bridge per edge connection
- Handles `EdgeMessage_Event` messages from edges
- Exposes `GetEventBus()` for other AI Studio components

### Microgateway (Edge Client)

The `SimpleEdgeClient` in `microgateway/internal/grpc/simple_client.go`:
- Creates an event bus on startup
- Creates a bridge when connecting to control
- Handles `ControlMessage_Event` messages from control
- Exposes `GetEventBus()` for other Microgateway components

## Proto Definition

Events are transmitted using `EventFrame` in `proto/config_sync.proto`:

```protobuf
message EventFrame {
  string id = 1;
  string topic = 2;
  string origin = 3;
  int32 dir = 4;    // 0 = Local, 1 = Up, 2 = Down
  bytes payload = 5;
}
```
