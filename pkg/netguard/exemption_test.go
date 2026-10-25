package netguard

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExemptionFor_Hosts(t *testing.T) {
	e := ExemptionFor(true, "http://Ollama:11434/v1", "localhost:5432", "redis", "", "postgres://u:p@db.internal:5432/x", "https://ollama:11434")
	for _, h := range []string{"ollama", "localhost", "redis", "db.internal", "OLLAMA"} {
		if !e.Has(h) {
			t.Errorf("Has(%q) = false", h)
		}
	}
	if got := e.Hosts(); len(got) != 4 {
		t.Errorf("Hosts() = %v, want 4 unique hosts", got)
	}
	if ExemptionFor(false, "http://ollama:11434").Has("ollama") {
		t.Error("a switch that is off exempts its host")
	}
	if (Exemption{}).Has("localhost") {
		t.Error("the zero Exemption exempts a host")
	}
}

// An asset's exemption lets its own host be internal and nothing else: the
// same internal server reached as another host is still refused, and the
// global allowlist still applies.
func TestExemption_OnlyTheAssetsHosts(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvAllowInternal, "")
	t.Setenv(EnvAllowedInternalHosts, "")
	setBlockInternal(t, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	port := mustURL(t, srv.URL).Port()

	asset := ExemptionFor(true, "http://localhost:"+port)
	client := asset.HTTPClient(5 * time.Second)
	resp, err := client.Get("http://localhost:" + port)
	if err != nil {
		t.Fatalf("the asset's own internal host was refused: %v", err)
	}
	resp.Body.Close()
	if err := asset.ValidateURL(mustURL(t, "http://localhost:"+port)); err != nil {
		t.Errorf("ValidateURL refused the asset's host: %v", err)
	}

	if _, err := client.Get(srv.URL); err == nil {
		t.Error("the asset's client reached another internal host (127.0.0.1)")
	}
	if err := asset.ValidateURL(mustURL(t, srv.URL)); err == nil {
		t.Error("ValidateURL let another internal IP through")
	}
	other := ExemptionFor(false, "http://localhost:"+port).HTTPClient(5 * time.Second)
	if _, err := other.Get("http://localhost:" + port); err == nil {
		t.Error("an asset without the switch reached the same internal host")
	}

	// The switch never bypasses the global allowlist.
	t.Setenv(EnvAllowedHosts, "api.example.com")
	if err := asset.ValidateURL(mustURL(t, "http://localhost:"+port)); err == nil {
		t.Error("the switch bypassed LLM_UPSTREAM_ALLOWED_HOSTS")
	}
}

func TestExemption_DialContext(t *testing.T) {
	t.Setenv(EnvAllowInternal, "")
	t.Setenv(EnvAllowedInternalHosts, "")
	setBlockInternal(t, "true")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	if _, err := (Exemption{}).DialContext(context.Background(), "tcp", "localhost:"+port); err == nil {
		t.Error("an unexempted dial to localhost succeeded")
	}
	c, err := ExemptionFor(true, "localhost:"+port).DialContext(context.Background(), "tcp", "localhost:"+port)
	if err != nil {
		t.Fatalf("the exempted dial failed: %v", err)
	}
	c.Close()
}

// A transport shared by many assets pools connections per exemption: a
// connection dialled for an exempted asset is never reused for a request
// without the exemption, whose dial is refused.
func TestPooledByExemption_NoReuseAcrossExemptions(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	t.Setenv(EnvAllowInternal, "")
	t.Setenv(EnvAllowedInternalHosts, "")
	setBlockInternal(t, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	target := "http://localhost:" + mustURL(t, srv.URL).Port()

	client := &http.Client{Transport: PooledByExemption(func(e Exemption) http.RoundTripper { return e.Transport() })}
	get := func(ctx context.Context) error {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		return err
	}
	exempt := ExemptionFor(true, target).WithContext(context.Background())
	if err := get(exempt); err != nil {
		t.Fatalf("the exempted request failed: %v", err)
	}
	if err := get(context.Background()); err == nil {
		t.Fatal("a request without the exemption reused the exempted connection")
	}
	if err := get(exempt); err != nil {
		t.Fatalf("the exempted request failed the second time: %v", err)
	}
}
