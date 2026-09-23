package scenario

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/seed"
)

func fullEnv() Env {
	return Env{
		Targets: map[string]Target{
			"gateway":   {BaseURL: "http://gw", Headers: map[string]string{"Authorization": "Bearer s"}},
			"mock":      {BaseURL: "http://mock"},
			"openai":    {BaseURL: "https://api.openai.com/v1"},
			"anthropic": {BaseURL: "https://api.anthropic.com/v1"},
		},
		State: &seed.State{Models: map[string]string{"mock": "mock-model", "openai": "gpt-x", "anthropic": "claude-x"}},
	}
}

// Every shipped scenario must parse strictly and resolve.
func TestShippedScenariosResolve(t *testing.T) {
	files, err := filepath.Glob("../../scenarios/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenarios found: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			s, err := Load(f)
			if err != nil {
				t.Fatal(err)
			}
			cells, skipped, err := s.Resolve(fullEnv())
			if err != nil {
				t.Fatal(err)
			}
			if len(skipped) > 0 || len(cells) != len(s.Cells) {
				t.Fatalf("skipped %v", skipped)
			}
			if s.Mode == "open" {
				if _, err := s.Open.RateFunc(); err != nil {
					t.Fatal(err)
				}
			}
			for _, c := range cells {
				for _, a := range c.Arms {
					if strings.Contains(a.Target.URL, "{") {
						t.Fatalf("unexpanded placeholder in %s", a.Target.URL)
					}
				}
			}
			s.ApplyQuick()
		})
	}
}

func TestResolveExpandsPlaceholdersAndArmModel(t *testing.T) {
	s, err := Load("../../scenarios/s3-real-vendors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cells, _, err := s.Resolve(fullEnv())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]int{}
	for i, c := range cells {
		byName[c.Name] = i
	}

	// The YAML merge key carries the base body; the override replaces extra.
	var body map[string]any
	_ = json.Unmarshal(cells[byName["openai-stream-512"]].Request.Body, &body)
	if body["model"] != "gpt-x" || body["max_completion_tokens"] != float64(512) || body["reasoning_effort"] != "none" {
		t.Fatalf("openai-stream-512 body: %v", body)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Fatal("max_tokens should be removed for the OpenAI reasoning model")
	}

	// The unified arm gets a slug-qualified model; the direct arm keeps the bare one.
	uni := cells[byName["openai-unified-stream"]]
	for _, a := range uni.Arms {
		var b map[string]any
		req := uni.Request
		if a.Request != nil {
			req = *a.Request
		}
		_ = json.Unmarshal(req.Body, &b)
		want := "gpt-x"
		if a.Name == "gateway" {
			want = "bench-openai/gpt-x"
		}
		if b["model"] != want {
			t.Fatalf("arm %s model = %v, want %s", a.Name, b["model"], want)
		}
	}

	// Mock profiles are URL-escaped into the path.
	s1, _ := Load("../../scenarios/s1-overhead-floor.yaml")
	c1, _, _ := s1.Resolve(fullEnv())
	if got := c1[0].Arms[1].Target.URL; got != "http://mock/p/tokens=16/v1/chat/completions" {
		t.Fatalf("direct URL %s", got)
	}
}

func TestResolveSkipsCellsWithoutTargets(t *testing.T) {
	s, _ := Load("../../scenarios/s3-real-vendors.yaml")
	env := fullEnv()
	delete(env.Targets, "anthropic")
	cells, skipped, err := s.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 3 || len(cells) != 4 {
		t.Fatalf("cells=%d skipped=%v", len(cells), skipped)
	}
}

func TestScaleRateAndQuick(t *testing.T) {
	s, _ := Load("../../scenarios/s7-soak.yaml")
	s.ScaleRate(2)
	if s.Open.Rate.RPS != 280 {
		t.Fatal(s.Open.Rate.RPS)
	}
	s.ApplyQuick()
	if s.Open.Rate.Duration != 3*time.Minute {
		t.Fatalf("quick soak duration %v", s.Open.Rate.Duration)
	}
}
