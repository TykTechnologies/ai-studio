package chat_session

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgresForQueueTest opens its own pool against DATABASE_URL, or skips.
func openPostgresForQueueTest(t *testing.T, maxOpen int) *gorm.DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL queue test")
	}
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("Failed to connect to PostgreSQL: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get SQL database: %v", err)
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// createQueueWithin fails the test instead of hanging when queue creation
// blocks; a blocked creation is exactly the bug these tests guard against.
func createQueueWithin(t *testing.T, factory QueueFactory, sessionID string, limit time.Duration) (MessageQueue, error) {
	t.Helper()
	type result struct {
		q   MessageQueue
		err error
	}
	done := make(chan result, 1)
	go func() {
		q, err := factory.CreateQueue(sessionID, nil)
		done <- result{q, err}
	}()
	select {
	case r := <-done:
		return r.q, r.err
	case <-time.After(limit):
		t.Fatalf("creating queue %s blocked for more than %s", sessionID, limit)
		return nil, nil
	}
}

func testQueueConfig(bufferSize int, notifyTimeout time.Duration) PostgreSQLConfig {
	return PostgreSQLConfig{
		BufferSize:          bufferSize,
		ReconnectInterval:   time.Second,
		MaxReconnectRetries: 3,
		NotifyTimeout:       notifyTimeout,
	}
}

// Sessions on the shared factory must not hold pool connections: many more
// sessions than MaxOpenConns are created, and each one still delivers.
func TestSharedPostgreSQLQueue_ManySessionsSmallPool(t *testing.T) {
	db := openPostgresForQueueTest(t, 2)
	sqlDB, _ := db.DB()
	factory := NewSharedPostgreSQLQueueFactory(db, testQueueConfig(20, 2*time.Second))

	const sessions = 12
	prefix := fmt.Sprintf("many-%d", time.Now().UnixNano())
	queues := make([]MessageQueue, 0, sessions)
	defer func() {
		for _, q := range queues {
			q.Close()
		}
	}()
	for i := 0; i < sessions; i++ {
		q, err := createQueueWithin(t, factory, fmt.Sprintf("%s-%d", prefix, i), 15*time.Second)
		if err != nil {
			t.Fatalf("session %d: %v", i, err)
		}
		queues = append(queues, q)
	}

	if inUse := sqlDB.Stats().InUse; inUse != 0 {
		t.Errorf("sessions pin %d pool connections while idle, want 0", inUse)
	}

	// Server side, one backend holds every session's LISTEN.
	var listeners int
	if err := db.Raw(`SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND query ILIKE 'LISTEN %'`).Scan(&listeners).Error; err != nil {
		t.Fatal(err)
	}
	if listeners != 1 {
		t.Errorf("%d sessions use %d LISTEN connections, want 1", sessions, listeners)
	}

	for i, q := range queues {
		msg := &ChatResponse{Payload: fmt.Sprintf("hello %d", i)}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := q.PublishMessage(ctx, msg); err != nil {
			cancel()
			t.Fatalf("session %d publish: %v", i, err)
		}
		cancel()
		select {
		case got := <-q.ConsumeMessages(context.Background()):
			if got == nil || got.Payload != msg.Payload {
				t.Errorf("session %d received %+v, want payload %q", i, got, msg.Payload)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("session %d never received its message", i)
		}
	}
}

// Every message type round-trips through the shared (production) queue, and
// sessions only see their own messages.
func TestSharedPostgreSQLQueue_DeliversAllTypes(t *testing.T) {
	db := openPostgresForQueueTest(t, 4)
	factory := NewSharedPostgreSQLQueueFactory(db, testQueueConfig(10, 2*time.Second))
	prefix := fmt.Sprintf("types-%d", time.Now().UnixNano())

	a, err := createQueueWithin(t, factory, prefix+"-a", 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := createQueueWithin(t, factory, prefix+"-b", 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.PublishMessage(ctx, &ChatResponse{Payload: "msg-a"}); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishStream(ctx, []byte("stream-a")); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishError(ctx, fmt.Errorf("err-a")); err != nil {
		t.Fatal(err)
	}

	select {
	case m := <-a.ConsumeMessages(ctx):
		if m.Payload != "msg-a" {
			t.Errorf("message payload %q", m.Payload)
		}
	case <-ctx.Done():
		t.Fatal("no chat response delivered")
	}
	select {
	case s := <-a.ConsumeStream(ctx):
		if string(s) != "stream-a" {
			t.Errorf("stream %q", s)
		}
	case <-ctx.Done():
		t.Fatal("no stream data delivered")
	}
	select {
	case e := <-a.ConsumeErrors(ctx):
		if e == nil || e.Error() != "err-a" {
			t.Errorf("error %v", e)
		}
	case <-ctx.Done():
		t.Fatal("no error delivered")
	}

	select {
	case m := <-b.ConsumeMessages(context.Background()):
		t.Errorf("session b received session a's message: %+v", m)
	case <-time.After(300 * time.Millisecond):
	}
}

// Two queues for the same session in one process both receive, and closing
// one does not silence the other.
func TestSharedPostgreSQLQueue_SameSessionTwice(t *testing.T) {
	db := openPostgresForQueueTest(t, 4)
	factory := NewSharedPostgreSQLQueueFactory(db, testQueueConfig(10, 2*time.Second))
	sid := fmt.Sprintf("twice-%d", time.Now().UnixNano())

	first, err := createQueueWithin(t, factory, sid, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createQueueWithin(t, factory, sid, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := first.PublishMessage(ctx, &ChatResponse{Payload: "one"}); err != nil {
		t.Fatal(err)
	}
	for name, q := range map[string]MessageQueue{"first": first, "second": second} {
		select {
		case <-q.ConsumeMessages(ctx):
		case <-ctx.Done():
			t.Fatalf("%s queue did not receive", name)
		}
	}

	first.Close()
	if err := second.PublishMessage(ctx, &ChatResponse{Payload: "two"}); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-second.ConsumeMessages(ctx):
		if m.Payload != "two" {
			t.Errorf("payload %q", m.Payload)
		}
	case <-ctx.Done():
		t.Fatal("closing one queue silenced the other queue for the same session")
	}
}

// The shared listener reconnects after its backend is killed, re-issues
// LISTEN, and sessions receive again.
func TestSharedPostgreSQLQueue_ListenerReconnects(t *testing.T) {
	db := openPostgresForQueueTest(t, 4)
	factory := NewSharedPostgreSQLQueueFactory(db, testQueueConfig(50, 2*time.Second))
	q, err := createQueueWithin(t, factory, fmt.Sprintf("reconnect-%d", time.Now().UnixNano()), 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	var killed int
	if err := db.Raw(`SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND query ILIKE 'LISTEN %'`).Scan(&killed).Error; err != nil {
		t.Fatal(err)
	}
	if killed != 1 {
		t.Fatalf("terminated %d listener backends, want 1", killed)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := q.PublishMessage(ctx, &ChatResponse{Payload: "after reconnect"})
		cancel()
		if err == nil {
			select {
			case <-q.ConsumeMessages(context.Background()):
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	t.Fatal("no message delivered after the listener's backend was terminated")
}

// When the pool is exhausted, creating a queue fails with an error within the
// notify timeout instead of blocking forever.
func TestSharedPostgreSQLQueue_FailsFastWhenPoolExhausted(t *testing.T) {
	db := openPostgresForQueueTest(t, 1)
	sqlDB, _ := db.DB()
	held, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	factory := NewSharedPostgreSQLQueueFactory(db, testQueueConfig(10, 500*time.Millisecond))
	start := time.Now()
	q, err := createQueueWithin(t, factory, fmt.Sprintf("exhausted-%d", time.Now().UnixNano()), 10*time.Second)
	if err == nil {
		q.Close()
		t.Fatal("expected an error while the pool is exhausted")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("creation took %s to fail, want about the notify timeout", elapsed)
	}
}

// Publishing with a context that has no deadline is bounded by NotifyTimeout.
func TestSharedPostgreSQLQueue_PublishBoundedWithoutDeadline(t *testing.T) {
	db := openPostgresForQueueTest(t, 1)
	sqlDB, _ := db.DB()
	factory := NewSharedPostgreSQLQueueFactory(db, testQueueConfig(10, 500*time.Millisecond))
	q, err := createQueueWithin(t, factory, fmt.Sprintf("bounded-%d", time.Now().UnixNano()), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	held, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	done := make(chan error, 1)
	go func() { done <- q.PublishMessage(context.Background(), &ChatResponse{Payload: "x"}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the publish to fail while the pool is exhausted")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("publish without a deadline blocked on an exhausted pool")
	}
}
