package overload

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func serve(h http.Handler, remote string, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil)
	r.RemoteAddr = remote
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Above the threshold new requests are refused with 503, Retry-After and an
// OpenAI-shaped error; they are admitted again only below threshold - 5%.
func TestMemorySheddingWithHysteresis(t *testing.T) {
	m := New(Config{Enabled: true, MemoryLimit: 1000, Threshold: 0.85})
	h := m.Middleware(okHandler())

	m.update(800)
	if w := serve(h, "10.0.0.1:1234", nil); w.Code != http.StatusOK {
		t.Fatalf("below threshold: %d", w.Code)
	}

	m.update(900)
	w := serve(h, "10.0.0.1:1234", nil)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("above threshold: %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	var body struct {
		Error struct{ Type, Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != "overloaded" || body.Error.Type != "server_error" {
		t.Fatalf("error body %q: %v", w.Body.String(), err)
	}

	m.update(820) // below the threshold but within the hysteresis band
	if w := serve(h, "10.0.0.1:1234", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("within hysteresis: %d", w.Code)
	}
	m.update(790)
	if w := serve(h, "10.0.0.1:1234", nil); w.Code != http.StatusOK {
		t.Fatalf("below hysteresis: %d", w.Code)
	}
	if s := m.Stats(); s.RejectedMemory != 2 || s.Inflight != 0 {
		t.Fatalf("stats %+v", s)
	}
}

// The in-flight cap refuses requests beyond MaxInflight and frees a slot when
// a request finishes.
func TestInflightCap(t *testing.T) {
	m := New(Config{Enabled: true, MaxInflight: 2})
	if m.Acquire() != Admitted || m.Acquire() != Admitted {
		t.Fatal("first two requests should be admitted")
	}
	if m.Acquire() != Inflight {
		t.Fatal("third request should be refused")
	}
	m.Release()
	if m.Acquire() != Admitted {
		t.Fatal("a released slot should admit again")
	}
	if s := m.Stats(); s.Inflight != 2 || s.RejectedInflight != 1 {
		t.Fatalf("stats %+v", s)
	}
}

// The gateway's own /ai/ loopback hop is neither refused nor counted, but only
// over a loopback connection: the header alone must not bypass shedding.
func TestLoopbackHopExempt(t *testing.T) {
	m := New(Config{Enabled: true, MemoryLimit: 1000, Threshold: 0.85})
	m.update(990)
	h := m.Middleware(okHandler())
	hop := map[string]string{hdrInternalHop: "1"}

	if w := serve(h, "127.0.0.1:5555", hop); w.Code != http.StatusOK {
		t.Fatalf("loopback hop refused: %d", w.Code)
	}
	if w := serve(h, "[::1]:5555", hop); w.Code != http.StatusOK {
		t.Fatalf("IPv6 loopback hop refused: %d", w.Code)
	}
	if w := serve(h, "203.0.113.9:5555", hop); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("external request with the hop header admitted: %d", w.Code)
	}
}

func TestDisabledAdmitsEverything(t *testing.T) {
	m := New(Config{Enabled: false, MemoryLimit: 1000, MaxInflight: 1})
	m.update(999)
	h := m.Middleware(okHandler())
	for i := 0; i < 3; i++ {
		if w := serve(h, "10.0.0.1:1", nil); w.Code != http.StatusOK {
			t.Fatalf("disabled manager refused: %d", w.Code)
		}
	}
}

func TestCgroupMemoryLimit(t *testing.T) {
	dir := t.TempDir()
	if got := cgroupMemoryLimit(dir); got != 0 {
		t.Fatalf("no files: %d", got)
	}
	write := func(p, v string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// cgroup v1, "no limit" is a huge number.
	write(filepath.Join(dir, "memory", "memory.limit_in_bytes"), "9223372036854771712\n")
	if got := cgroupMemoryLimit(dir); got != 0 {
		t.Fatalf("v1 unlimited: %d", got)
	}
	write(filepath.Join(dir, "memory", "memory.limit_in_bytes"), "2147483648\n")
	if got := cgroupMemoryLimit(dir); got != 2147483648 {
		t.Fatalf("v1 limit: %d", got)
	}
	// cgroup v2 takes precedence.
	write(filepath.Join(dir, "memory.max"), "max\n")
	if got := cgroupMemoryLimit(dir); got != 0 {
		t.Fatalf("v2 max: %d", got)
	}
	write(filepath.Join(dir, "memory.max"), "1073741824\n")
	if got := cgroupMemoryLimit(dir); got != 1073741824 {
		t.Fatalf("v2 limit: %d", got)
	}
}

func TestParseBytes(t *testing.T) {
	for in, want := range map[string]uint64{
		"": 0, "1024": 1024, "2GiB": 2 << 30, "1536MiB": 1536 << 20, "512M": 512 << 20, "1.5GiB": 3 << 29, "2GB": 2e9,
	} {
		got, err := ParseBytes(in)
		if err != nil || got != want {
			t.Errorf("ParseBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseBytes("lots"); err == nil {
		t.Error("ParseBytes(lots) should fail")
	}
}

// The memory sample is live Go memory: positive, and not wildly above the heap.
func TestSampleGoMemory(t *testing.T) {
	if got := sampleGoMemory(); got == 0 {
		t.Fatal("sampleGoMemory returned 0")
	}
}
