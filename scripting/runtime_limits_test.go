//go:build enterprise

package scripting

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/enterprise/scriptExtensions/httpcaller"
	ent_scripting "github.com/TykTechnologies/midsommar/v2/enterprise/scripting"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// The limits are owned by the enterprise runtime; alias them so the tests read
// as documentation of the operator-facing names.
const (
	EnvScriptTimeoutName        = ent_scripting.EnvScriptTimeout
	EnvScriptMaxAllocsName      = ent_scripting.EnvScriptMaxAllocs
	EnvScriptAllowOSName        = ent_scripting.EnvScriptAllowOS
	EnvHTTPTimeoutName          = httpcaller.EnvHTTPTimeout
	EnvHTTPMaxResponseBytesName = httpcaller.EnvHTTPMaxResponseBytes
)

// These tests pin the runtime bounds on filter scripts: wall time, allocations,
// the modules a script may import, and the guards on the tyk.makeHTTPRequest
// and tyk.llm helpers. Filters run on the goroutine serving the request, so
// each bound is the difference between one failed request and a stuck worker.

func runSource(t *testing.T, src string, input *ScriptInput, svc services.ServiceInterface) (*ScriptOutput, error) {
	t.Helper()
	if input == nil {
		input = &ScriptInput{RawInput: `{"x":1}`, VendorName: "openai", ModelName: "gpt-4"}
	}
	return NewScriptRunner([]byte(src)).RunScript(input, svc)
}

func TestRunScript_TimesOut(t *testing.T) {
	t.Setenv(EnvScriptTimeoutName, "200ms")
	t.Setenv(EnvScriptMaxAllocsName, "0")

	start := time.Now()
	_, err := runSource(t, `for true { }`, nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("RunScript() returned no error for a script that never terminates")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("RunScript() error = %q, want it to say the script timed out", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("RunScript() took %s to abort a 200ms-limited script", elapsed)
	}
}

func TestRunScript_HonoursCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runSource(t, `output := {block: false, payload: input.raw_input, message: ""}`,
		&ScriptInput{RawInput: `{}`, Ctx: ctx}, nil)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("RunScript() error = %v, want a cancellation error for an already-cancelled context", err)
	}
}

func TestRunScript_AllocationLimit(t *testing.T) {
	t.Setenv(EnvScriptMaxAllocsName, "1000")

	_, err := runSource(t, `
		items := []
		for i := 0; i < 100000; i++ {
			items = append(items, {n: i})
		}
		output := {block: false, payload: input.raw_input, message: ""}
	`, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "allocation") {
		t.Fatalf("RunScript() error = %v, want the allocation limit to be reported", err)
	}
}

func TestRunScript_OSModuleGated(t *testing.T) {
	src := `
		os := import("os")
		output := {block: false, payload: input.raw_input, message: "os available"}
	`

	t.Run("blocked by default", func(t *testing.T) {
		_, err := runSource(t, src, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "os") {
			t.Fatalf("RunScript() error = %v, want the os module to be unavailable", err)
		}
	})

	t.Run("available when FILTER_SCRIPT_ALLOW_OS=true", func(t *testing.T) {
		t.Setenv(EnvScriptAllowOSName, "true")
		out, err := runSource(t, src, nil, nil)
		if err != nil {
			t.Fatalf("RunScript() error = %v", err)
		}
		if out.Message != "os available" {
			t.Errorf("RunScript() Message = %q", out.Message)
		}
	})

	t.Run("pure modules stay available", func(t *testing.T) {
		out, err := runSource(t, `
			text := import("text")
			json := import("json")
			math := import("math")
			times := import("times")
			base64 := import("base64")
			output := {block: false, payload: text.to_upper("ok"), message: ""}
		`, nil, nil)
		if err != nil {
			t.Fatalf("RunScript() error = %v", err)
		}
		if out.Payload != "OK" {
			t.Errorf("RunScript() Payload = %q", out.Payload)
		}
	})
}

// The compiled form of a script is cached and each run executes on a clone.
// Fifty concurrent runs of one source with distinct inputs must each see only
// their own input.
func TestRunScript_CachedCompileIsolatesRuns(t *testing.T) {
	src := `output := {block: false, payload: input.raw_input + "|" + input.model_name, message: ""}`

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw := fmt.Sprintf("r%d", i)
			out, err := runSource(t, src, &ScriptInput{RawInput: raw, ModelName: "m"}, nil)
			if err != nil {
				errs <- err
				return
			}
			if want := raw + "|m"; out.Payload != want {
				errs <- fmt.Errorf("run %d: Payload = %q, want %q", i, out.Payload, want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestMakeHTTPRequest_Guards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			fmt.Fprintf(w, `{"auth":%q}`, r.Header.Get("Authorization"))
		case "/big":
			w.Write(bytes.Repeat([]byte("a"), 4096))
		case "/slow":
			time.Sleep(1 * time.Second)
			w.Write([]byte("late"))
		case "/redirect":
			http.Redirect(w, r, "ftp://example.invalid/x", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	script := func(url string) string {
		return fmt.Sprintf(`
			tyk := import("tyk")
			res := tyk.makeHTTPRequest("GET", %q, {"Authorization": "Bearer t"}, "")
			output := {block: false, payload: res.response, message: string(res.status)}
		`, url)
	}

	t.Run("call goes through with headers and status", func(t *testing.T) {
		out, err := runSource(t, script(srv.URL+"/echo"), nil, nil)
		if err != nil {
			t.Fatalf("RunScript() error = %v", err)
		}
		if !strings.Contains(out.Payload, `"auth":"Bearer t"`) || out.Message != "200" {
			t.Errorf("RunScript() payload=%q message=%q", out.Payload, out.Message)
		}
	})

	t.Run("response body is capped", func(t *testing.T) {
		t.Setenv(EnvHTTPMaxResponseBytesName, "1024")
		_, err := runSource(t, script(srv.URL+"/big"), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("RunScript() error = %v, want the response cap to be reported", err)
		}
	})

	t.Run("call is bounded in time", func(t *testing.T) {
		t.Setenv(EnvHTTPTimeoutName, "200ms")
		start := time.Now()
		_, err := runSource(t, script(srv.URL+"/slow"), nil, nil)
		if err == nil {
			t.Fatal("RunScript() returned no error for a call that outlives FILTER_HTTP_TIMEOUT")
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("RunScript() took %s, the HTTP timeout did not apply", elapsed)
		}
	})

	t.Run("non-http scheme is refused", func(t *testing.T) {
		_, err := runSource(t, script("ftp://example.invalid/x"), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("RunScript() error = %v, want the scheme to be refused", err)
		}
	})

	t.Run("redirect to a refused destination fails", func(t *testing.T) {
		_, err := runSource(t, script(srv.URL+"/redirect"), nil, nil)
		if err == nil {
			t.Fatal("RunScript() returned no error for a redirect to a non-http scheme")
		}
	})

	t.Run("host allowlist applies", func(t *testing.T) {
		t.Setenv("LLM_UPSTREAM_ALLOWED_HOSTS", "api.example.invalid")
		_, err := runSource(t, script(srv.URL+"/echo"), nil, nil)
		if err == nil || !strings.Contains(err.Error(), "not in") {
			t.Fatalf("RunScript() error = %v, want the host to be refused by the allowlist", err)
		}
	})
}

// llmStubService serves one LLM for tyk.llm; every other method of the
// interface is unreachable in these tests.
type llmStubService struct {
	services.ServiceInterface
	llm models.LLM
}

func (s *llmStubService) GetLLMByID(id uint) (*models.LLM, error) {
	cp := s.llm
	return &cp, nil
}

// tyk.llm must send the resolved credential, not the "$ENV/..." or
// "$SECRET/..." reference the LLM row stores.
func TestLLMCall_ResolvesSecretReference(t *testing.T) {
	t.Setenv("FILTER_TEST_LLM_KEY", "resolved-key")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","model":"gpt-4o-mini",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"yes"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	svc := &llmStubService{llm: models.LLM{
		Vendor:      models.Vendor("openai"),
		APIKey:      "$ENV/FILTER_TEST_LLM_KEY",
		APIEndpoint: srv.URL,
	}}

	out, err := runSource(t, `
		tyk := import("tyk")
		res := tyk.llm(1, {model_name: "gpt-4o-mini", max_tokens: 5}, "is this ok?")
		output := {block: res == "yes", payload: input.raw_input, message: res}
	`, nil, svc)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if gotAuth != "Bearer resolved-key" {
		t.Errorf("upstream saw Authorization %q, want the resolved key", gotAuth)
	}
	if !out.Block || out.Message != "yes" {
		t.Errorf("RunScript() block=%v message=%q, want the LLM verdict to drive the output", out.Block, out.Message)
	}
}
