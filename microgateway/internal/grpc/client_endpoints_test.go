package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// attemptLog records, in order, which control plane each registration
// attempt reached and when.
type attemptLog struct {
	mu      sync.Mutex
	entries []attempt
}

type attempt struct {
	name string
	at   time.Time
}

func (l *attemptLog) add(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, attempt{name, time.Now()})
}

func (l *attemptLog) snapshot() []attempt {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]attempt(nil), l.entries...)
}

func (l *attemptLog) names() []string {
	var out []string
	for _, a := range l.snapshot() {
		out = append(out, a.name)
	}
	return out
}

// siteControl is one site's control plane: it can refuse registrations, drop
// the streams it holds, and hold a token validation open.
type siteControl struct {
	pb.UnimplementedConfigurationSyncServiceServer
	name   string
	log    *attemptLog
	reject atomic.Bool

	registrations atomic.Int32
	streams       atomic.Int32 // opened in total
	open          atomic.Int32 // open now

	mu    sync.Mutex
	drops []context.CancelFunc

	// validating is closed when a ValidateToken call has arrived; the call
	// answers when release is closed.
	validating chan struct{}
	release    chan struct{}
	validOnce  sync.Once
}

func newSiteControl(name string, log *attemptLog) *siteControl {
	return &siteControl{name: name, log: log, validating: make(chan struct{}), release: make(chan struct{})}
}

func (s *siteControl) RegisterEdge(ctx context.Context, req *pb.EdgeRegistrationRequest) (*pb.EdgeRegistrationResponse, error) {
	if s.log != nil {
		s.log.add(s.name)
	}
	if s.reject.Load() {
		return nil, status.Error(codes.Unavailable, "site is down")
	}
	s.registrations.Add(1)
	return &pb.EdgeRegistrationResponse{Success: true, SessionId: s.name}, nil
}

func (s *siteControl) SubscribeToChanges(stream grpc.BidiStreamingServer[pb.EdgeMessage, pb.ControlMessage]) error {
	ctx, cancel := context.WithCancel(stream.Context())
	s.mu.Lock()
	s.drops = append(s.drops, cancel)
	s.mu.Unlock()
	s.streams.Add(1)
	s.open.Add(1)
	defer s.open.Add(-1)
	errs := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				errs <- err
				return
			}
		}
	}()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return status.Error(codes.Unavailable, "stream dropped")
	}
}

func (s *siteControl) ValidateToken(ctx context.Context, req *pb.TokenValidationRequest) (*pb.TokenValidationResponse, error) {
	s.validOnce.Do(func() { close(s.validating) })
	select {
	case <-s.release:
		return &pb.TokenValidationResponse{Valid: true, AppId: 7, AppName: s.name}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// dropStreams ends every stream the site holds, as a failing site would. It
// first waits for the stream the edge opened to reach the site's handler:
// the edge counts itself connected a moment before that.
func (s *siteControl) dropStreams(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return s.open.Load() > 0 }, 5*time.Second, 5*time.Millisecond, "site %s holds no stream to drop", s.name)
	s.mu.Lock()
	drops := s.drops
	s.drops = nil
	s.mu.Unlock()
	for _, drop := range drops {
		drop()
	}
}

// serve starts the site on addr ("127.0.0.1:0" for any port) and returns
// its address and server.
func (s *siteControl) serve(t *testing.T, addr string, opts ...grpc.ServerOption) (string, *grpc.Server) {
	t.Helper()
	var lis net.Listener
	var err error
	require.Eventually(t, func() bool {
		lis, err = net.Listen("tcp", addr)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond, "listen on %s", addr)
	srv := grpc.NewServer(opts...)
	pb.RegisterConfigurationSyncServiceServer(srv, s)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), srv
}

// closedAddr returns a loopback address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

func endpointsClient(t *testing.T, interval time.Duration, mutate func(*config.HubSpokeConfig), endpoints ...string) *SimpleEdgeClient {
	t.Helper()
	hs := config.HubSpokeConfig{
		EdgeID:            "endpoints-edge",
		EdgeNamespace:     "test",
		ControlEndpoints:  endpoints,
		AllowInsecure:     true,
		HeartbeatInterval: time.Hour,
		ReconnectInterval: interval,
	}
	if mutate != nil {
		mutate(&hs)
	}
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: hs}, "test", "hash", "time")
	t.Cleanup(func() { _ = client.Stop() })
	return client
}

func waitLink(t *testing.T, client *SimpleEdgeClient, endpoint string) {
	t.Helper()
	require.Eventually(t, func() bool {
		link := client.ControlLink()
		return link.Connected && link.Endpoint == endpoint
	}, 10*time.Second, 10*time.Millisecond, "edge never connected to %s (on %+v)", endpoint, client.ControlLink())
}

// Start takes the first endpoint that accepts the edge, in list order.
func TestControlEndpoints_StartUsesTheFirstThatAnswers(t *testing.T) {
	a, b := newSiteControl("a", nil), newSiteControl("b", nil)
	addrA, _ := a.serve(t, "127.0.0.1:0")
	addrB, _ := b.serve(t, "127.0.0.1:0")

	t.Run("first is up", func(t *testing.T) {
		client := endpointsClient(t, time.Second, nil, addrA, addrB)
		require.NoError(t, client.Start())
		waitLink(t, client, addrA)
		assert.EqualValues(t, 0, b.registrations.Load(), "the second endpoint is not contacted while the first works")
		assert.Equal(t, []string{addrA, addrB}, client.ControlLink().Endpoints)
	})

	t.Run("first is down", func(t *testing.T) {
		client := endpointsClient(t, time.Second, nil, closedAddr(t), addrB)
		require.NoError(t, client.Start())
		waitLink(t, client, addrB)
	})

	t.Run("first refuses the registration", func(t *testing.T) {
		refusing := newSiteControl("refusing", nil)
		refusing.reject.Store(true)
		addr, _ := refusing.serve(t, "127.0.0.1:0")
		client := endpointsClient(t, time.Second, nil, addr, addrB)
		require.NoError(t, client.Start())
		waitLink(t, client, addrB)
	})

	t.Run("none answers", func(t *testing.T) {
		client := endpointsClient(t, time.Second, nil, closedAddr(t), closedAddr(t))
		require.Error(t, client.Start())
		assert.False(t, client.ControlLink().Connected)
	})

	t.Run("CONTROL_ENDPOINT is a list of one", func(t *testing.T) {
		client := endpointsClient(t, time.Second, func(hs *config.HubSpokeConfig) {
			hs.ControlEndpoints = nil
			hs.ControlEndpoint = addrA
		})
		require.NoError(t, client.Start())
		waitLink(t, client, addrA)
		assert.Equal(t, []string{addrA}, client.ControlLink().Endpoints)
	})
}

// When the stream to one control plane ends, the edge goes to the next one
// before any backoff: with a 3 s reconnect interval it is on B well inside
// that interval.
func TestControlEndpoints_FailsOverBeforeBackingOff(t *testing.T) {
	a, b := newSiteControl("a", nil), newSiteControl("b", nil)
	addrA, srvA := a.serve(t, "127.0.0.1:0")
	addrB, _ := b.serve(t, "127.0.0.1:0")

	client := endpointsClient(t, 3*time.Second, nil, addrA, addrB)
	require.NoError(t, client.Start())
	waitLink(t, client, addrA)

	cut := time.Now()
	srvA.Stop()
	waitLink(t, client, addrB)
	assert.Less(t, time.Since(cut), 2*time.Second, "failover waited for the backoff")
	require.Eventually(t, func() bool { return b.open.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	// Token validation uses the connection to B from here on.
	close(b.release)
	resp, err := client.ValidateTokenOnDemand("tok")
	require.NoError(t, err)
	assert.Equal(t, "b", resp.AppName)
}

// With every control plane refusing, the edge tries the other endpoints at
// once, then backs off, and from then on makes one pass over the list (from
// its first entry) per backoff.
func TestControlEndpoints_BackoffAppliesPerPass(t *testing.T) {
	const interval = 300 * time.Millisecond
	log := &attemptLog{}
	a, b, c := newSiteControl("a", log), newSiteControl("b", log), newSiteControl("c", log)
	addrA, _ := a.serve(t, "127.0.0.1:0")
	addrB, _ := b.serve(t, "127.0.0.1:0")
	addrC, _ := c.serve(t, "127.0.0.1:0")

	client := endpointsClient(t, interval, nil, addrA, addrB, addrC)
	require.NoError(t, client.Start())
	waitLink(t, client, addrA)
	require.Equal(t, []string{"a"}, log.names())

	// Every site refuses registrations, and A drops the edge.
	a.reject.Store(true)
	b.reject.Store(true)
	c.reject.Store(true)
	a.dropStreams(t)

	// a (start) | b c (the others, at once) | a b c (first pass) | a b c
	want := []string{"a", "b", "c", "a", "b", "c", "a", "b", "c"}
	require.Eventually(t, func() bool { return len(log.names()) >= len(want) }, 10*time.Second, 10*time.Millisecond)
	got := log.snapshot()[:len(want)]
	var names []string
	for _, e := range got {
		names = append(names, e.name)
	}
	require.Equal(t, want, names)

	gap := func(i int) time.Duration { return got[i].at.Sub(got[i-1].at) }
	// Inside a pass the endpoints follow each other without a wait.
	for _, i := range []int{2, 4, 5, 7, 8} {
		assert.Less(t, gap(i), interval/2, "attempt %d (%s) waited %s inside a pass", i, names[i], gap(i))
	}
	// Between passes the backoff applies: the base interval (less jitter),
	// then double it.
	assert.GreaterOrEqual(t, gap(3), interval*9/10, "first backoff")
	assert.GreaterOrEqual(t, gap(6), interval*2*9/10, "second backoff doubles")

	// A site that recovers is found on the next pass.
	b.reject.Store(false)
	waitLink(t, client, addrB)
}

// An edge with one endpoint behaves as before: after a lost stream it waits
// out the reconnect interval before it tries again.
func TestControlEndpoints_SingleEndpointStillBacksOffFirst(t *testing.T) {
	const interval = 400 * time.Millisecond
	log := &attemptLog{}
	a := newSiteControl("a", log)
	addrA, _ := a.serve(t, "127.0.0.1:0")

	client := endpointsClient(t, interval, func(hs *config.HubSpokeConfig) {
		hs.ControlEndpoints = nil
		hs.ControlEndpoint = addrA
	})
	require.NoError(t, client.Start())
	waitLink(t, client, addrA)

	dropped := time.Now()
	a.dropStreams(t)
	require.Eventually(t, func() bool { return len(log.names()) >= 2 }, 10*time.Second, 10*time.Millisecond)
	again := log.snapshot()[1].at
	assert.GreaterOrEqual(t, again.Sub(dropped), interval*9/10, "a single endpoint is retried only after the reconnect interval")
	waitLink(t, client, addrA)
}

// With EDGE_CONTROL_FAILBACK_INTERVAL set, an edge on its second endpoint
// returns to the first once that accepts registrations again, and a call in
// flight on the connection it leaves still completes.
func TestControlEndpoints_FailbackReturnsToTheFirst(t *testing.T) {
	a, b := newSiteControl("a", nil), newSiteControl("b", nil)
	addrA := closedAddr(t) // site A is down when the edge starts
	addrB, _ := b.serve(t, "127.0.0.1:0")

	client := endpointsClient(t, 50*time.Millisecond, func(hs *config.HubSpokeConfig) {
		hs.ControlFailbackInterval = 100 * time.Millisecond
	}, addrA, addrB)
	require.NoError(t, client.Start())
	waitLink(t, client, addrB)

	// A token validation is in flight on B when the edge moves.
	type result struct {
		resp *pb.TokenValidationResponse
		err  error
	}
	inFlight := make(chan result, 1)
	go func() {
		resp, err := client.ValidateTokenOnDemand("tok-in-flight")
		inFlight <- result{resp, err}
	}()
	select {
	case <-b.validating:
	case <-time.After(5 * time.Second):
		t.Fatal("the validation never reached B")
	}

	// Site A comes back.
	a.serve(t, addrA)
	waitLink(t, client, addrA)
	require.Eventually(t, func() bool { return a.open.Load() == 1 && b.open.Load() == 0 }, 5*time.Second, 10*time.Millisecond,
		"the stream moved: A open=%d, B open=%d", a.open.Load(), b.open.Load())

	// The call on B's connection was not cut by the move.
	select {
	case r := <-inFlight:
		t.Fatalf("the in-flight validation ended during the move: %+v %v", r.resp, r.err)
	case <-time.After(200 * time.Millisecond):
	}
	close(b.release)
	select {
	case r := <-inFlight:
		require.NoError(t, r.err)
		assert.Equal(t, "b", r.resp.AppName)
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight validation never answered")
	}

	// New calls go to A, and the edge stays there.
	close(a.release)
	resp, err := client.ValidateTokenOnDemand("tok-after")
	require.NoError(t, err)
	assert.Equal(t, "a", resp.AppName)
	registrations := a.registrations.Load()
	time.Sleep(350 * time.Millisecond)
	assert.Equal(t, registrations, a.registrations.Load(), "an edge on its first endpoint does not probe it")
	assert.EqualValues(t, 1, a.streams.Load())

	// The stream the failback ended asks for a reconnection as any ended
	// stream does. That request can arrive after the move is complete (it
	// did, under load: the edge then dropped its new connection and
	// connected again). It must find the edge connected and do nothing.
	client.attemptReconnection()
	time.Sleep(100 * time.Millisecond)
	link := client.ControlLink()
	assert.True(t, link.Connected)
	assert.Equal(t, addrA, link.Endpoint)
	assert.EqualValues(t, 1, a.streams.Load(), "a late reconnection request replaced the stream the failback opened")
	assert.Equal(t, registrations, a.registrations.Load())
}

// Without the failback interval the edge stays where it is after the first
// endpoint returns, until that connection ends.
func TestControlEndpoints_NoFailbackByDefault(t *testing.T) {
	a, b := newSiteControl("a", nil), newSiteControl("b", nil)
	addrA := closedAddr(t)
	addrB, _ := b.serve(t, "127.0.0.1:0")

	client := endpointsClient(t, 50*time.Millisecond, nil, addrA, addrB)
	require.NoError(t, client.Start())
	waitLink(t, client, addrB)

	a.serve(t, addrA)
	time.Sleep(500 * time.Millisecond)
	assert.EqualValues(t, 0, a.registrations.Load(), "the edge contacted its first endpoint without a failback interval")
	link := client.ControlLink()
	assert.True(t, link.Connected)
	assert.Equal(t, addrB, link.Endpoint)

	// When the connection to B ends, the edge goes to the next endpoint in
	// the list after B, which is A again.
	b.dropStreams(t)
	waitLink(t, client, addrA)
}

// A failback that finds the first endpoint still refusing leaves the edge
// connected where it is.
func TestControlEndpoints_FailbackProbeFailureKeepsTheConnection(t *testing.T) {
	a, b := newSiteControl("a", nil), newSiteControl("b", nil)
	a.reject.Store(true)
	addrA, _ := a.serve(t, "127.0.0.1:0")
	addrB, _ := b.serve(t, "127.0.0.1:0")

	client := endpointsClient(t, 50*time.Millisecond, func(hs *config.HubSpokeConfig) {
		hs.ControlFailbackInterval = 50 * time.Millisecond
	}, addrA, addrB)
	require.NoError(t, client.Start())
	waitLink(t, client, addrB)

	time.Sleep(400 * time.Millisecond) // several probes
	link := client.ControlLink()
	assert.True(t, link.Connected)
	assert.Equal(t, addrB, link.Endpoint)
	assert.EqualValues(t, 1, b.streams.Load(), "the stream to B was not disturbed by failed probes")

	a.reject.Store(false)
	waitLink(t, client, addrA)
}

// siteCerts issues, from a new CA, one server certificate per list of DNS
// names and IP addresses, and writes the CA to a file.
func siteCerts(t *testing.T, sans ...[]string) (caPath string, certs []tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "endpoints test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	caPath = filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))

	for i, names := range sans {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(int64(i + 2)),
			Subject:      pkix.Name{CommonName: fmt.Sprintf("site-%d", i)},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		for _, name := range names {
			if ip := net.ParseIP(name); ip != nil {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			} else {
				tmpl.DNSNames = append(tmpl.DNSNames, name)
			}
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		require.NoError(t, err)
		certs = append(certs, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key})
	}
	return caPath, certs
}

// With TLS, each endpoint's certificate is checked against that endpoint's
// own host; EDGE_TLS_SERVER_NAME, when set, replaces the host for all of
// them. One CA covers every endpoint.
func TestControlEndpoints_TLSNameFollowsTheEndpoint(t *testing.T) {
	// Site A's certificate is for the name "localhost" only, site B's for
	// the address 127.0.0.1 only.
	caPath, certs := siteCerts(t, []string{"localhost"}, []string{"127.0.0.1"})
	a, b := newSiteControl("a", nil), newSiteControl("b", nil)
	addrA, _ := a.serve(t, "127.0.0.1:0", grpc.Creds(credentials.NewServerTLSFromCert(&certs[0])))
	addrB, _ := b.serve(t, "127.0.0.1:0", grpc.Creds(credentials.NewServerTLSFromCert(&certs[1])))
	_, portA, err := net.SplitHostPort(addrA)
	require.NoError(t, err)
	endpointA := net.JoinHostPort("localhost", portA)

	withTLS := func(serverName string) func(*config.HubSpokeConfig) {
		return func(hs *config.HubSpokeConfig) {
			hs.AllowInsecure = false
			hs.ClientTLSEnabled = true
			hs.ClientTLSCAPath = caPath
			hs.ClientTLSServerName = serverName
		}
	}

	t.Run("each endpoint is checked against its own host", func(t *testing.T) {
		client := endpointsClient(t, 50*time.Millisecond, withTLS(""), endpointA, addrB)
		require.NoError(t, client.Start())
		waitLink(t, client, endpointA)
		a.dropStreams(t) // B is next
		waitLink(t, client, addrB)
	})

	t.Run("a certificate for another endpoint's host is refused", func(t *testing.T) {
		// B's certificate (127.0.0.1) does not cover "localhost".
		_, portB, err := net.SplitHostPort(addrB)
		require.NoError(t, err)
		client := endpointsClient(t, 50*time.Millisecond, withTLS(""), net.JoinHostPort("localhost", portB))
		require.Error(t, client.Start())
	})

	t.Run("a failover checks the next endpoint against its own host", func(t *testing.T) {
		// The second endpoint is B reached as "localhost", a name its
		// certificate (127.0.0.1 only) does not carry: the edge must not
		// carry A's name over, and must not accept B there.
		_, portB, err := net.SplitHostPort(addrB)
		require.NoError(t, err)
		wrongB := net.JoinHostPort("localhost", portB)
		before := b.registrations.Load()
		client := endpointsClient(t, 50*time.Millisecond, withTLS(""), endpointA, wrongB)
		require.NoError(t, client.Start())
		waitLink(t, client, endpointA)
		a.dropStreams(t)
		// It cannot land on B under the wrong name; it comes back to A.
		require.Eventually(t, func() bool { return a.streams.Load() >= 2 && client.ControlLink().Connected }, 10*time.Second, 10*time.Millisecond)
		assert.Equal(t, endpointA, client.ControlLink().Endpoint)
		assert.Equal(t, before, b.registrations.Load(), "B was accepted under a name its certificate does not carry")
	})

	t.Run("EDGE_TLS_SERVER_NAME overrides the host across a failover", func(t *testing.T) {
		// Two sites whose certificates both carry one shared name and not
		// the addresses the edge dials them by.
		sharedCA, shared := siteCerts(t, []string{"cp.internal"}, []string{"cp.internal"})
		c, d := newSiteControl("c", nil), newSiteControl("d", nil)
		addrC, _ := c.serve(t, "127.0.0.1:0", grpc.Creds(credentials.NewServerTLSFromCert(&shared[0])))
		addrD, _ := d.serve(t, "127.0.0.1:0", grpc.Creds(credentials.NewServerTLSFromCert(&shared[1])))
		tlsWith := func(name string) func(*config.HubSpokeConfig) {
			return func(hs *config.HubSpokeConfig) {
				hs.AllowInsecure = false
				hs.ClientTLSEnabled = true
				hs.ClientTLSCAPath = sharedCA
				hs.ClientTLSServerName = name
			}
		}

		// Without the override neither is accepted by address.
		require.Error(t, endpointsClient(t, 50*time.Millisecond, tlsWith(""), addrC, addrD).Start())

		client := endpointsClient(t, 50*time.Millisecond, tlsWith("cp.internal"), addrC, addrD)
		require.NoError(t, client.Start())
		waitLink(t, client, addrC)
		c.reject.Store(true)
		c.dropStreams(t)
		waitLink(t, client, addrD) // the name applies to the endpoint it failed over to
		// A name neither certificate carries is refused on both.
		require.Error(t, endpointsClient(t, 50*time.Millisecond, tlsWith("other.internal"), addrC, addrD).Start())
	})

	t.Run("EDGE_TLS_SERVER_NAME applies to every endpoint", func(t *testing.T) {
		// With the name fixed to "localhost", A is accepted by address and
		// B, whose certificate does not carry that name, is not.
		before := b.registrations.Load()
		client := endpointsClient(t, 50*time.Millisecond, withTLS("localhost"), addrB, addrA)
		require.NoError(t, client.Start())
		waitLink(t, client, addrA)
		assert.Equal(t, before, b.registrations.Load())
	})
}

// An endpoint that does not answer at all must not hold an edge that has
// another one to go to for gRPC's 20 s connect timeout.
func TestControlEndpoints_SilentEndpointIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the connect timeout")
	}
	// A listener that accepts connections and never speaks.
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { silent.Close() })
	var held []net.Conn
	var mu sync.Mutex
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			c.Close()
		}
	})

	b := newSiteControl("b", nil)
	addrB, _ := b.serve(t, "127.0.0.1:0")
	client := endpointsClient(t, 50*time.Millisecond, nil, silent.Addr().String(), addrB)
	start := time.Now()
	require.NoError(t, client.Start())
	waitLink(t, client, addrB)
	assert.Less(t, time.Since(start), controlConnectTimeout+3*time.Second)
}
