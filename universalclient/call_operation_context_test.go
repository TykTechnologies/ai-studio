package universalclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tool calls used to be made with context.Background(): a closed chat session
// could not cancel an in-flight tool call, and no trace context reached the
// tool. CallOperationWithContext binds the request to the caller's context and
// injects the W3C trace headers.
func TestCallOperationWithContext(t *testing.T) {
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	var seenTraceparent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTraceparent = r.Header.Get("Traceparent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": 1, "name": "doggie", "status": "available"}})
	}))
	defer server.Close()

	client, err := NewClient(specBytes, server.URL+"/v2")
	require.NoError(t, err)

	t.Run("cancelled context aborts the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.CallOperationWithContext(ctx, "findPetsByStatus",
			map[string][]string{"status": {"available"}}, nil, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("trace context is propagated to the tool", func(t *testing.T) {
		// The global propagator is what InjectOutgoing uses; the default is a
		// no-op, so install the W3C one for the test and restore it after.
		prev := otel.GetTextMapPropagator()
		otel.SetTextMapPropagator(propagation.TraceContext{})
		defer otel.SetTextMapPropagator(prev)

		sc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
			TraceFlags: trace.FlagsSampled,
		})
		ctx := trace.ContextWithSpanContext(context.Background(), sc)

		_, err := client.CallOperationWithContext(ctx, "findPetsByStatus",
			map[string][]string{"status": {"available"}}, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, "00-0102030405060708090a0b0c0d0e0f10-0102030405060708-01", seenTraceparent)
	})

	t.Run("nil context is tolerated", func(t *testing.T) {
		_, err := client.CallOperationWithContext(nil, "findPetsByStatus", //nolint:staticcheck // the nil tolerance is the point
			map[string][]string{"status": {"available"}}, nil, nil)
		require.NoError(t, err)
	})
}
