package plugins

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

// The plugin broker's interceptor records the served plugin under
// BoundPluginIDKey, which the management server checks every request against.
func TestBoundPluginIDInterceptor(t *testing.T) {
	var got interface{}
	_, err := boundPluginIDInterceptor(42)(context.Background(), nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			got = ctx.Value(BoundPluginIDKey)
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := got.(uint32); !ok || id != 42 {
		t.Fatalf("BoundPluginIDKey = %v (%T), want uint32 42", got, got)
	}
}
