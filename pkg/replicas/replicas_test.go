package replicas

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeBackend struct {
	leader  bool
	sent    []string
	sendErr error
}

func (f *fakeBackend) IsLeader() bool { return f.leader }
func (f *fakeBackend) Signal(_ context.Context, name string) error {
	f.sent = append(f.sent, name)
	return f.sendErr
}

func TestWithoutBackendThisProcessIsTheOnlyReplica(t *testing.T) {
	SetBackend(nil)
	assert.True(t, IsLeader())
	Signal(context.Background(), "x") // goes nowhere, no panic
}

func TestSignalsAndHandlers(t *testing.T) {
	b := &fakeBackend{}
	SetBackend(b)
	t.Cleanup(func() { SetBackend(nil) })
	assert.False(t, IsLeader())

	calls := 0
	remove := OnSignal("budgets", func() { calls++ })
	OnSignal("budgets", func() { panic("a broken handler") }) // does not stop the others
	other := 0
	OnSignal("governed_metadata", func() { other++ })

	Signal(context.Background(), "budgets")
	assert.Equal(t, []string{"budgets"}, b.sent)
	assert.Zero(t, calls, "the signalling replica's own handlers do not run")

	Deliver("budgets")
	assert.Equal(t, 1, calls)
	assert.Zero(t, other)

	remove()
	Deliver("budgets")
	assert.Equal(t, 1, calls, "removed")

	b.sendErr = errors.New("database is down")
	Signal(context.Background(), "budgets") // logged, not returned
}

func TestOnLeading(t *testing.T) {
	calls := 0
	remove := OnLeading(func() { calls++ })
	t.Cleanup(OnLeading(func() { panic("a broken handler") })) // does not stop the others

	BecameLeader()
	assert.Equal(t, 1, calls)
	remove()
	BecameLeader()
	assert.Equal(t, 1, calls, "removed")
}
