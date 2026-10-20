package pluginlimits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestMaxMsgBytes(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want int
	}{
		{"", DefaultMaxMsgBytes},
		{"  ", DefaultMaxMsgBytes},
		{"1048576", 1 << 20},
		{" 134217728 ", 128 << 20},
		{"0", DefaultMaxMsgBytes},
		{"-5", DefaultMaxMsgBytes},
		{"64MiB", DefaultMaxMsgBytes},
		{"99999999999", math.MaxInt32},
	} {
		t.Setenv(EnvMaxMsgBytes, tc.env)
		if got := MaxMsgBytes(); got != tc.want {
			t.Errorf("%s=%q: MaxMsgBytes() = %d, want %d", EnvMaxMsgBytes, tc.env, got, tc.want)
		}
	}
}

// echoPlugin answers post_auth with the body it was sent.
type echoPlugin struct {
	pb.UnimplementedPluginServiceServer
}

func (echoPlugin) ProcessPostAuth(_ context.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	return &pb.PluginResponse{Modified: true, Body: req.GetRequest().GetBody()}, nil
}

// dial serves echoPlugin on srv and returns a client dialled the way
// go-plugin dials plugins (no client-side limit).
func dial(t *testing.T, srv *grpc.Server) pb.PluginServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	pb.RegisterPluginServiceServer(srv, echoPlugin{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt32), grpc.MaxCallSendMsgSize(math.MaxInt32)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewPluginServiceClient(conn)
}

func bigRequest(n int) *pb.EnrichedRequest {
	return &pb.EnrichedRequest{Request: &pb.PluginRequest{Body: bytes.Repeat([]byte("a"), n)}}
}

func TestGRPCServerAcceptsLargeMessages(t *testing.T) {
	t.Setenv(EnvMaxMsgBytes, "")
	client := dial(t, GRPCServer(nil))
	resp, err := client.ProcessPostAuth(context.Background(), bigRequest(20<<20))
	if err != nil {
		t.Fatalf("20 MiB post_auth: %v", err)
	}
	if len(resp.Body) != 20<<20 {
		t.Fatalf("echoed %d bytes, want %d", len(resp.Body), 20<<20)
	}
}

// The env override lowers the limit as well as raising it.
func TestGRPCServerHonoursEnvOverride(t *testing.T) {
	t.Setenv(EnvMaxMsgBytes, "1048576")
	client := dial(t, GRPCServer(nil))
	if _, err := client.ProcessPostAuth(context.Background(), bigRequest(512<<10)); err != nil {
		t.Fatalf("512 KiB under a 1 MiB limit: %v", err)
	}
	_, err := client.ProcessPostAuth(context.Background(), bigRequest(2<<20))
	if !IsTooLarge(err) {
		t.Fatalf("2 MiB over a 1 MiB limit: err = %v, want a too-large error", err)
	}
}

// A plugin on an older SDK (go-plugin's DefaultGRPCServer) refuses 5 MiB, and
// IsTooLarge recognises the refusal.
func TestLegacyServerRefusalIsTooLarge(t *testing.T) {
	client := dial(t, grpc.NewServer())
	_, err := client.ProcessPostAuth(context.Background(), bigRequest(5<<20))
	if err == nil {
		t.Fatal("a default gRPC server accepted 5 MiB")
	}
	if !IsTooLarge(err) {
		t.Fatalf("IsTooLarge(%v) = false", err)
	}
	wrapped := AsTooLarge(err, "old-cache", "post_auth", 5<<20)
	tl, ok := AsTooLargeError(wrapped)
	if !ok {
		t.Fatalf("AsTooLarge did not wrap: %v", wrapped)
	}
	if tl.Plugin != "old-cache" || tl.Hook != "post_auth" || tl.Bytes != 5<<20 {
		t.Errorf("TooLargeError = %+v", tl)
	}
	if !strings.Contains(wrapped.Error(), "old-cache") || !strings.Contains(wrapped.Error(), EnvMaxMsgBytes) {
		t.Errorf("message does not name the plugin and the setting: %s", wrapped)
	}
	if status.Code(errors.Unwrap(wrapped)) != codes.ResourceExhausted {
		t.Error("TooLargeError does not unwrap to the gRPC status")
	}
}

func TestIsTooLargeIgnoresOtherErrors(t *testing.T) {
	for _, err := range []error{
		nil,
		errors.New("received message larger than max"),
		status.Error(codes.ResourceExhausted, "rate limit exceeded"),
		status.Error(codes.Internal, "received message larger than max (5 vs. 4)"),
	} {
		if IsTooLarge(err) {
			t.Errorf("IsTooLarge(%v) = true", err)
		}
		if AsTooLarge(err, "p", "post_auth", 1) != err {
			t.Errorf("AsTooLarge wrapped %v", err)
		}
	}
}

func TestCheckSize(t *testing.T) {
	t.Setenv(EnvMaxMsgBytes, "100")
	if err := CheckSize("p", "post_auth", 100); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	err := CheckSize("cache", "post_auth", 101)
	tl, ok := AsTooLargeError(err)
	if !ok || !errors.Is(err, ErrRequestTooLarge) || tl.Plugin != "cache" || tl.Bytes != 101 {
		t.Fatalf("over the limit: %v (%+v)", err, tl)
	}
	if !IsRequestTooLarge(err) || !strings.Contains(err.Error(), "100-byte plugin limit") {
		t.Errorf("err = %v", err)
	}
	if got := ClientMessage(err); got != "request too large for the gateway's plugins (101 bytes)" || strings.Contains(got, "cache") {
		t.Errorf("ClientMessage = %q", got)
	}
	plain := fmt.Errorf("body: %w", ErrRequestTooLarge)
	if !IsRequestTooLarge(plain) || ClientMessage(plain) != "request too large for the gateway's plugins" {
		t.Errorf("plain ErrRequestTooLarge: %v, %q", IsRequestTooLarge(plain), ClientMessage(plain))
	}
	if IsRequestTooLarge(errors.New("other")) || IsRequestTooLarge(nil) {
		t.Error("IsRequestTooLarge matched an unrelated error")
	}
}

// The host's service-API servers accept HostServiceMaxMsgBytes from plugins,
// whatever the body limit is set to.
func TestHostServiceServerOptions(t *testing.T) {
	t.Setenv(EnvMaxMsgBytes, "")
	client := dial(t, grpc.NewServer(HostServiceServerOptions()...))
	if _, err := client.ProcessPostAuth(context.Background(), bigRequest(6<<20)); err != nil {
		t.Fatalf("6 MiB: %v", err)
	}
	if _, err := client.ProcessPostAuth(context.Background(), bigRequest(HostServiceMaxMsgBytes+1)); !IsTooLarge(err) {
		t.Fatalf("over HostServiceMaxMsgBytes: err = %v", err)
	}
}
