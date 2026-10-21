package requestid

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestNew_ShapeAndUniqueness(t *testing.T) {
	const n = 20000
	var mu sync.Mutex
	seen := make(map[string]bool, n)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n/8; i++ {
				id := New()
				if !Valid(id) {
					t.Errorf("New() = %q is not Valid", id)
					return
				}
				mu.Lock()
				if seen[id] {
					t.Errorf("duplicate id %q", id)
				}
				seen[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("got %d distinct ids, want %d", len(seen), n)
	}
}

func TestNew_OrderedByTime(t *testing.T) {
	a := New()
	b := New()
	// UUIDv7's first 48 bits are the millisecond: a later id never sorts
	// before an earlier one by more than the same millisecond's random part.
	if a[len(Prefix):len(Prefix)+12] > b[len(Prefix):len(Prefix)+12] {
		t.Fatalf("time prefix went backwards: %s then %s", a, b)
	}
}

func TestValid(t *testing.T) {
	for _, bad := range []string{
		"", "req_1792561677381285034", "tyai_", "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1", // one short
		"tyai_0192D8A4F6B37C1E9A5B2D4C6E8F0A1B", // upper case is not what New makes
		"tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1g", "tyai_0192d8a4-f6b37c1e9a5b2d4c6e8f0a1", "xxxx_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b",
		"tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b\n",
	} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true", bad)
		}
	}
	if !Valid("tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b") {
		t.Error("a well-formed id is not Valid")
	}
}

func TestContext(t *testing.T) {
	ctx := WithID(context.Background(), "tyai_x", "client-1")
	if FromContext(ctx) != "tyai_x" || ClientFromContext(ctx) != "client-1" {
		t.Fatalf("got %q / %q", FromContext(ctx), ClientFromContext(ctx))
	}
	// Plugins and hooks read the plain string key.
	if v, _ := ctx.Value("request_id").(string); v != "tyai_x" {
		t.Fatalf(`ctx.Value("request_id") = %q`, v)
	}
	if ClientFromContext(WithID(context.Background(), "tyai_y", "")) != "" {
		t.Fatal("an empty client id was stored")
	}
	if FromContext(nil) != "" || ClientFromContext(context.Background()) != "" { //nolint:staticcheck // nil is the case under test
		t.Fatal("ids from an empty context")
	}
}

func TestSanitizeClient(t *testing.T) {
	cases := map[string]string{
		"abc-123_x.y:z/w":         "abc-123_x.y:z/w",
		"ab c\n<script>\"'":       "abcscript",
		"Root=1-5f84c7a6-abc;x=1": "Root1-5f84c7a6-abcx1",
		strings.Repeat("a", 300):  strings.Repeat("a", MaxClientLen),
		"été-日本":                  "t-",
	}
	for in, want := range cases {
		if got := SanitizeClient(in); got != want {
			t.Errorf("SanitizeClient(%q) = %q, want %q", in, got, want)
		}
	}
}
