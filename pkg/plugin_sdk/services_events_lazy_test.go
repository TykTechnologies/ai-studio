package plugin_sdk

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	pb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
)

// recordingEventClient records published topics in order.
type recordingEventClient struct {
	pb.PluginEventServiceClient
	mu     sync.Mutex
	topics []string
}

func (c *recordingEventClient) Publish(_ context.Context, in *pb.PublishRequest, _ ...grpc.CallOption) (*pb.PublishResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topics = append(c.topics, in.Topic)
	return &pb.PublishResponse{Success: true}, nil
}

// lateBroker is a connect func that fails until ready is set.
type lateBroker struct {
	ready  bool
	client *recordingEventClient
}

func (b *lateBroker) connect() (pb.PluginEventServiceClient, error) {
	if !b.ready {
		return nil, errors.New("broker not ready")
	}
	return b.client, nil
}

// Events published before the broker is ready are held and sent, in order,
// once it is; the service is not stuck on a stub after the first failure.
func TestLazyEventService_BuffersUntilBrokerIsReady(t *testing.T) {
	broker := &lateBroker{client: &recordingEventClient{}}
	svc := &lazyEventService{pluginID: "7", connect: broker.connect}
	ctx := context.Background()

	for _, topic := range []string{"a", "b", "c"} {
		if err := svc.Publish(ctx, topic, map[string]string{"k": topic}, DirLocal); err != nil {
			t.Fatalf("Publish(%s) before the broker is ready: %v", topic, err)
		}
	}
	if _, err := svc.Subscribe("x", func(Event) {}); err == nil {
		t.Error("Subscribe succeeded before the broker was ready")
	}
	if len(broker.client.topics) != 0 {
		t.Fatalf("events reached the host before the broker was ready: %v", broker.client.topics)
	}

	broker.ready = true
	svc.mu.Lock()
	svc.nextConnect = svc.nextConnect.AddDate(-1, 0, 0) // skip the retry wait
	svc.mu.Unlock()
	if err := svc.Publish(ctx, "d", "payload", DirLocal); err != nil {
		t.Fatalf("Publish after the broker is ready: %v", err)
	}

	want := []string{"a", "b", "c", "d"}
	got := broker.client.topics
	if len(got) != len(want) {
		t.Fatalf("host received %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host received %v, want %v in order", got, want)
		}
	}
}

// A slow connection attempt does not stall other publishers: they buffer
// and return, and everything is delivered in order once it connects.
func TestLazyEventService_SlowConnectDoesNotBlockPublishers(t *testing.T) {
	client := &recordingEventClient{}
	release := make(chan struct{})
	entered := make(chan struct{})
	svc := &lazyEventService{pluginID: "7", connect: func() (pb.PluginEventServiceClient, error) {
		close(entered)
		<-release
		return client, nil
	}}
	ctx := context.Background()

	first := make(chan error, 1)
	go func() { first <- svc.Publish(ctx, "first", 1, DirLocal) }()
	<-entered // the first publisher is now dialling

	done := make(chan error, 1)
	go func() { done <- svc.Publish(ctx, "second", 2, DirLocal) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Publish: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a publisher was blocked by another's connection attempt")
	}

	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first Publish: %v", err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.topics) != 2 || client.topics[0] != "first" || client.topics[1] != "second" {
		t.Fatalf("host received %v, want [first second]", client.topics)
	}
}

// Past the buffer, Publish reports the drop instead of returning nil.
func TestLazyEventService_OverflowIsReported(t *testing.T) {
	broker := &lateBroker{client: &recordingEventClient{}}
	svc := &lazyEventService{pluginID: "7", connect: broker.connect}
	ctx := context.Background()

	for i := 0; i < maxPendingEvents; i++ {
		if err := svc.Publish(ctx, "busy", i, DirUp); err != nil {
			t.Fatalf("Publish %d within the buffer: %v", i, err)
		}
	}
	if err := svc.Publish(ctx, "busy", "one too many", DirUp); !errors.Is(err, errEventDropped) {
		t.Fatalf("Publish beyond the buffer = %v, want errEventDropped", err)
	}
	if _, warned := svc.dropWarned["busy"]; !warned {
		t.Error("the drop was not logged for its topic")
	}
}
