// Package fipssmoke is the runtime smoke test of FIPS 140-3 AI Studio and
// microgateway builds (epic midsommar-clean-8dy7, bead .7). It runs against
// a Studio hub and an edge that are already up, as tests/fipssmoke/run.sh
// starts them from the -fips images or packages, and skips without
// FIPS_SMOKE_STUDIO_URL.
//
// It checks:
//   - both report FIPS 140-3 mode on GET /health (enabled, module v1.0.0);
//   - the TLS posture of every listener (Studio HTTPS, the control gRPC
//     port, the edge's HTTPS): TLS 1.0 and 1.1, ChaCha20-Poly1305 and an
//     X25519-only key share are refused; P-256 with AES-GCM is accepted, on
//     TLS 1.2 and 1.3. Each refusal is checked against a permissive Go TLS
//     server first, so a refusal always comes from the server under test,
//     never from an openssl client that cannot make the offer;
//   - hub->edge config sync over TLS and one proxied LLM call through the
//     edge to the mock vendor (benchmarks/gateway/cmd/mockllm).
//
// Pointed at standard builds (FIPS_SMOKE_EXPECT=std), the /health checks
// expect FIPS mode off and the posture checks that FIPS mode makes
// (ChaCha20, X25519) are expected to fail: that is how the checks are
// shown to detect a regression.
package fipssmoke

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Environment set by run.sh.
const (
	envStudio  = "FIPS_SMOKE_STUDIO_URL"  // https://localhost:18080
	envGateway = "FIPS_SMOKE_GATEWAY_URL" // https://localhost:18081
	envTargets = "FIPS_SMOKE_TLS_TARGETS" // name=host:port,...
	envCA      = "FIPS_SMOKE_CA"          // CA certificate of every listener
	envCert    = "FIPS_SMOKE_CERT"        // server certificate and key, for the
	envKey     = "FIPS_SMOKE_KEY"         // permissive control server
	envMock    = "FIPS_SMOKE_MOCK_URL"    // mock vendor, as the edge reaches it
	envExpect  = "FIPS_SMOKE_EXPECT"      // fips (default) or std
)

func TestFIPSSmoke(t *testing.T) {
	studio := os.Getenv(envStudio)
	if studio == "" {
		t.Skipf("%s not set; run tests/fipssmoke/run.sh", envStudio)
	}
	fips := os.Getenv(envExpect) != "std"
	client := httpClient(t)

	t.Run("health", func(t *testing.T) {
		for name, base := range map[string]string{"studio": studio, "gateway": os.Getenv(envGateway)} {
			t.Run(name, func(t *testing.T) { checkHealth(t, client, base, fips) })
		}
	})

	t.Run("tls", func(t *testing.T) {
		control := startPermissiveServer(t)
		for _, target := range strings.Split(os.Getenv(envTargets), ",") {
			name, addr, ok := strings.Cut(target, "=")
			if !ok {
				t.Fatalf("%s: bad target %q", envTargets, target)
			}
			t.Run(name, func(t *testing.T) { checkTLSPosture(t, addr, control, fips) })
		}
	})

	t.Run("sync-and-proxy", func(t *testing.T) { checkSyncAndProxy(t, client, studio, os.Getenv(envGateway)) })
}

func httpClient(t *testing.T) *http.Client {
	pem, err := os.ReadFile(os.Getenv(envCA))
	if err != nil {
		t.Fatalf("%s: %v", envCA, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatalf("%s: no certificate", envCA)
	}
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:       jar,
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
	}
}

// --- /health -------------------------------------------------------------

func checkHealth(t *testing.T, client *http.Client, base string, fips bool) {
	var body struct {
		FIPS *struct {
			Enabled  bool   `json:"enabled"`
			Version  string `json:"version"`
			Enforced bool   `json:"enforced"`
		} `json:"fips"`
	}
	resp, err := client.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET %s/health: %v", base, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s/health: HTTP %d: %s", base, resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.FIPS == nil {
		t.Fatalf("GET %s/health: no fips object in %s (%v)", base, raw, err)
	}
	if fips {
		if !body.FIPS.Enabled || body.FIPS.Version != "v1.0.0" {
			t.Errorf("want FIPS 140-3 mode with module v1.0.0, got %s", raw)
		}
	} else if body.FIPS.Enabled {
		t.Errorf("a standard build reports FIPS mode: %s", raw)
	}
}

// --- TLS posture ---------------------------------------------------------

// offer is one openssl s_client ClientHello.
type offer struct {
	name string
	args []string
	// accept: the FIPS server must complete the handshake with exactly this
	// protocol and cipher. Otherwise it must refuse with an alert.
	accept       bool
	wantProtocol string
	wantCipher   string
	// fipsOnly: a standard build accepts this offer, so the check only
	// means something against a FIPS build (and fails against a standard
	// one, which is how it is shown to work).
	fipsOnly bool
}

var offers = []offer{
	{name: "tls1.0-refused", args: []string{"-tls1", "-cipher", "DEFAULT@SECLEVEL=0"}},
	{name: "tls1.1-refused", args: []string{"-tls1_1", "-cipher", "DEFAULT@SECLEVEL=0"}},
	{name: "tls1.2-chacha20-refused", args: []string{"-tls1_2", "-cipher", "ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305"}, fipsOnly: true},
	{name: "tls1.3-chacha20-refused", args: []string{"-tls1_3", "-ciphersuites", "TLS_CHACHA20_POLY1305_SHA256"}, fipsOnly: true},
	// X25519 is only offered as a TLS 1.3 key share: in TLS 1.2 an
	// X25519-only supported_groups also rules out the P-256 certificate, so
	// every server refuses it.
	{name: "tls1.3-x25519-only-refused", args: []string{"-tls1_3", "-groups", "X25519"}, fipsOnly: true},
	{name: "tls1.2-p256-aes-gcm-accepted", args: []string{"-tls1_2", "-groups", "P-256", "-cipher", "ECDHE-ECDSA-AES128-GCM-SHA256"},
		accept: true, wantProtocol: "TLSv1.2", wantCipher: "ECDHE-ECDSA-AES128-GCM-SHA256"},
	// CBC with HMAC-SHA1: in Go's standard defaults, not in FIPS mode. (The
	// control gRPC port refuses it in standard builds too: HTTP/2 forbids
	// CBC suites.)
	{name: "tls1.2-aes-cbc-sha1-refused", args: []string{"-tls1_2", "-groups", "P-256", "-cipher", "ECDHE-ECDSA-AES128-SHA"}, fipsOnly: true},
	{name: "tls1.3-p256-aes-gcm-accepted", args: []string{"-tls1_3", "-groups", "P-256", "-ciphersuites", "TLS_AES_128_GCM_SHA256"},
		accept: true, wantProtocol: "TLSv1.3", wantCipher: "TLS_AES_128_GCM_SHA256"},
}

// handshake runs openssl s_client against addr with o's offer and reports
// whether the handshake completed, the negotiated "Protocol, Cipher" line,
// and whether the server sent an alert.
func handshake(t *testing.T, addr string, o offer) (ok bool, negotiated string, serverAlert bool, out string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := append([]string{"s_client", "-connect", addr, "-servername", "localhost", "-alpn", "h2",
		"-CAfile", os.Getenv(envCA), "-verify_return_error"}, o.args...)
	cmd := exec.CommandContext(ctx, "openssl", args...)
	cmd.Stdin = strings.NewReader("")
	b, err := cmd.CombinedOutput()
	out = string(b)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "New, ") {
			negotiated = strings.TrimPrefix(line, "New, ")
		}
	}
	serverAlert = strings.Contains(out, "SSL alert number")
	ok = err == nil && negotiated != "" && !strings.HasPrefix(negotiated, "(NONE)")
	return ok, negotiated, serverAlert, out
}

func checkTLSPosture(t *testing.T, addr, control string, fips bool) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Fatal("openssl is needed for the TLS posture checks")
	}
	for _, o := range offers {
		t.Run(o.name, func(t *testing.T) {
			// The client must be able to make the offer: a permissive
			// server accepts it.
			if ok, _, _, out := handshake(t, control, o); !ok {
				t.Fatalf("openssl cannot make this offer (the permissive control server refused it), so the check would prove nothing:\n%s", out)
			}
			ok, negotiated, alert, out := handshake(t, addr, o)
			switch {
			case o.accept:
				want := o.wantProtocol + ", Cipher is " + o.wantCipher
				if !ok || negotiated != want {
					t.Errorf("want the handshake to complete with %q, got %q:\n%s", want, negotiated, out)
				}
			case ok && (fips || !o.fipsOnly):
				t.Errorf("the server accepted this offer (%s); FIPS 140-3 mode must refuse it:\n%s", negotiated, out)
			case ok:
				t.Errorf("accepted (%s), as a standard build does: this check fails against a non-FIPS server", negotiated)
			case !alert:
				t.Errorf("the handshake failed without an alert from the server; cannot tell who refused:\n%s", out)
			}
		})
	}
	// The hybrid post-quantum key share FIPS mode allows
	// (docs/site/docs/fips.md). OpenSSL offers it only from 3.5, so this
	// one offer comes from crypto/tls (this test binary is a standard build).
	t.Run("tls1.3-x25519mlkem768-accepted", func(t *testing.T) {
		pem, err := os.ReadFile(os.Getenv(envCA))
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, &tls.Config{
			RootCAs:          pool,
			ServerName:       "localhost",
			MinVersion:       tls.VersionTLS13,
			CurvePreferences: []tls.CurveID{tls.X25519MLKEM768},
			NextProtos:       []string{"h2"},
		})
		if err != nil {
			t.Fatalf("a TLS 1.3 X25519MLKEM768-only offer was refused: %v", err)
		}
		defer conn.Close()
		if got := conn.ConnectionState().CurveID; got != tls.X25519MLKEM768 {
			t.Errorf("negotiated key exchange %v, want X25519MLKEM768", got)
		}
	})
}

// startPermissiveServer starts a TLS server (this test binary's standard
// crypto/tls) that accepts every offer in offers, with the listeners'
// certificate, and returns its address.
func startPermissiveServer(t *testing.T) string {
	cert, err := tls.LoadX509KeyPair(os.Getenv(envCert), os.Getenv(envKey))
	if err != nil {
		t.Fatalf("%s/%s: %v", envCert, envKey, err)
	}
	var suites []uint16
	for _, s := range tls.CipherSuites() {
		suites = append(suites, s.ID)
	}
	for _, s := range tls.InsecureCipherSuites() {
		suites = append(suites, s.ID)
	}
	cfg := &tls.Config{
		Certificates:     []tls.Certificate{cert},
		MinVersion:       tls.VersionTLS10,
		CipherSuites:     suites,
		CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256, tls.CurveP384},
		NextProtos:       []string{"h2"},
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(10 * time.Second))
				_ = c.(*tls.Conn).Handshake()
				_, _ = io.Copy(io.Discard, c)
			}(c)
		}
	}()
	return ln.Addr().String()
}

// --- hub -> edge sync and a proxied call ----------------------------------

type studioAPI struct {
	t      *testing.T
	client *http.Client
	base   string
	apiKey string
	csrf   string
}

func (s *studioAPI) do(method, path string, in, out any, want ...int) {
	s.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.base+path, body)
	req.Header.Set("Content-Type", "application/json")
	// The CSRF check wants a same-origin Referer on HTTPS.
	req.Header.Set("Referer", s.base+"/")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	} else if s.csrf != "" {
		req.Header.Set("X-CSRF-Token", s.csrf)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	for _, w := range want {
		if resp.StatusCode == w {
			if out != nil {
				if err := json.Unmarshal(raw, out); err != nil {
					s.t.Fatalf("%s %s: decoding %s: %v", method, path, raw, err)
				}
			}
			return
		}
	}
	s.t.Fatalf("%s %s: HTTP %d, want %v: %s", method, path, resp.StatusCode, want, raw)
}

type resource struct {
	Data struct {
		ID         string          `json:"id"`
		Attributes json.RawMessage `json:"attributes"`
	} `json:"data"`
}

func checkSyncAndProxy(t *testing.T, client *http.Client, studio, gateway string) {
	s := &studioAPI{t: t, client: client, base: studio}

	// The first registered user is an admin; an API key exempts the rest
	// from CSRF.
	resp, err := client.Get(studio + "/csrf-token")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s.csrf = resp.Header.Get("X-CSRF-Token"); s.csrf == "" {
		t.Fatalf("GET /csrf-token: no X-CSRF-Token header (HTTP %d)", resp.StatusCode)
	}
	const email, password = "fips-smoke@example.com", "Fips-Smoke-Passw0rd!"
	s.do(http.MethodPost, "/auth/register", map[string]any{"data": map[string]any{"type": "users", "attributes": map[string]any{
		"email": email, "name": "FIPS Smoke", "password": password, "with_portal": true, "with_chat": true}}}, nil,
		http.StatusCreated, http.StatusOK)
	s.do(http.MethodPost, "/auth/login", map[string]any{"data": map[string]any{"type": "login", "attributes": map[string]any{
		"email": email, "password": password}}}, nil, http.StatusOK)
	var rolled struct {
		Data struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	s.do(http.MethodPost, "/common/me/api-key/roll", nil, &rolled, http.StatusOK)
	if rolled.Data.APIKey == "" {
		t.Fatal("no API key")
	}
	s.apiKey = rolled.Data.APIKey
	// /common/me answers with the resource at the top level, or in a {data}
	// envelope.
	var me struct {
		ID   string `json:"id"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	s.do(http.MethodGet, "/common/me", nil, &me, http.StatusOK)
	userID, err := strconv.Atoi(me.ID + me.Data.ID)
	if err != nil {
		t.Fatalf("/common/me: no user id: %v", err)
	}

	// An LLM on the mock vendor, and an app allowed to use it.
	var llm resource
	s.do(http.MethodPost, "/api/v1/llms", map[string]any{"data": map[string]any{"type": "llms", "attributes": map[string]any{
		"name": "fips-smoke-mock", "vendor": "openai", "api_endpoint": os.Getenv(envMock) + "/v1", "api_key": "sk-fips-smoke",
		"default_model": "gpt-4o-mini", "active": true, "privacy_score": 50}}}, &llm, http.StatusCreated)
	llmID, _ := strconv.Atoi(llm.Data.ID)
	var app resource
	s.do(http.MethodPost, "/api/v1/apps", map[string]any{"data": map[string]any{"type": "apps", "attributes": map[string]any{
		"name": "fips-smoke-app", "description": "FIPS smoke test", "user_id": userID, "llm_ids": []int{llmID}, "is_active": true}}},
		&app, http.StatusCreated)
	var appAttrs struct {
		CredentialID int `json:"credential_id"`
	}
	_ = json.Unmarshal(app.Data.Attributes, &appAttrs)
	s.do(http.MethodPost, "/api/v1/apps/"+app.Data.ID+"/activate-credential", nil, nil, http.StatusOK, http.StatusNoContent)
	var cred resource
	s.do(http.MethodGet, "/api/v1/credentials/"+strconv.Itoa(appAttrs.CredentialID), nil, &cred, http.StatusOK)
	var credAttrs struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(cred.Data.Attributes, &credAttrs)
	if credAttrs.Secret == "" {
		t.Fatal("the app credential has no secret")
	}

	// The edge registered over TLS; push the configuration to it.
	deadline := time.Now().Add(90 * time.Second)
	for {
		var st struct {
			Namespace struct {
				TotalEdges int `json:"total_edges"`
			} `json:"namespace"`
		}
		s.do(http.MethodGet, "/api/v1/sync/status/default", nil, &st, http.StatusOK)
		if st.Namespace.TotalEdges >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no edge registered with the hub over TLS")
		}
		time.Sleep(2 * time.Second)
	}
	// reload-all works in both editions (namespace reload is Enterprise).
	s.do(http.MethodPost, "/api/v1/edges/reload-all", map[string]any{}, nil, http.StatusOK, http.StatusAccepted)

	// One proxied call through the edge reaches the mock vendor once the
	// edge has the configuration.
	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"FIPS smoke"}]}`
	var last string
	for {
		req, _ := http.NewRequest(http.MethodPost, gateway+"/llm/call/fips-smoke-mock/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+credAttrs.Secret)
		resp, err := client.Do(req)
		if err == nil {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && bytes.Contains(raw, []byte(`"choices"`)) {
				return
			}
			last = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, raw)
		} else {
			last = err.Error()
		}
		if time.Now().After(deadline) {
			t.Fatalf("no successful proxied call through the edge: %s", last)
		}
		time.Sleep(2 * time.Second)
	}
}
