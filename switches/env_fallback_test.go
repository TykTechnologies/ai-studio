package switches

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
)

// envCanaries are vendor variables a client library would read when it is
// given an empty key or destination. None may reach a request.
var envCanaries = map[string]string{
	"OPENAI_API_KEY":           "canary-openai-key",
	"OPENAI_ORGANIZATION":      "canary-openai-org",
	"OPENAI_BASE_URL":          "http://canary-openai-base.invalid/v1",
	"OPENAI_API_BASE":          "http://canary-openai-apibase.invalid/v1",
	"ANTHROPIC_API_KEY":        "canary-anthropic-key",
	"GOOGLE_API_KEY":           "canary-google-key",
	"HF_TOKEN":                 "canary-hf-token",
	"HUGGINGFACEHUB_API_TOKEN": "canary-hf-legacy-token",
	"OLLAMA_HOST":              "http://canary-ollama.invalid:11434",
	"AWS_ACCESS_KEY_ID":        "AKIACANARYCANARY0000",
	"AWS_SECRET_ACCESS_KEY":    "canary-aws-secret",
}

func setEnvCanaries(t *testing.T) {
	t.Helper()
	for k, v := range envCanaries {
		t.Setenv(k, v)
	}
}

// recorder is an upstream chosen by the LLM's writer; it keeps every request
// it receives and refuses them.
type recorder struct {
	mu   sync.Mutex
	reqs []string
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dump, _ := httputil.DumpRequest(r, true)
	rec.mu.Lock()
	rec.reqs = append(rec.reqs, string(dump))
	rec.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"refused"}}`))
}

func (rec *recorder) requests() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]string(nil), rec.reqs...)
}

func assertNoCanary(t *testing.T, reqs []string) {
	t.Helper()
	for _, req := range reqs {
		for k, v := range envCanaries {
			if strings.Contains(req, v) {
				t.Errorf("the value of %s reached the upstream:\n%s", k, req)
			}
		}
	}
}

// A vendor client given an empty key, or the placeholder the hub sends for a
// refused reference, never fills it (or its destination) from the host
// environment, on the direct path and on the internal-routing path.
func TestVendorClientsNeverFallBackToEnvironment(t *testing.T) {
	setEnvCanaries(t)

	for _, vendor := range []models.Vendor{models.OPENAI, models.ANTHROPIC} {
		for _, internal := range []bool{false, true} {
			name := string(vendor)
			if internal {
				name += "/internal-routing"
			}
			t.Run(name, func(t *testing.T) {
				rec := &recorder{}
				srv := httptest.NewServer(rec)
				defer srv.Close()

				fetch := func(key string) (llms.Model, error) {
					conf := &models.LLM{Vendor: vendor, APIEndpoint: srv.URL + "/v1", APIKey: key}
					settings := &models.LLMSettings{ModelName: "m"}
					if internal {
						return FetchDriver(conf, settings, nil, nil, WithHTTPClient(srv.Client()))
					}
					return FetchDriver(conf, settings, nil, nil)
				}

				if llm, err := fetch(""); err == nil {
					_, _ = llms.GenerateFromSinglePrompt(context.Background(), llm, "hi")
					t.Errorf("an empty key built a driver")
				}
				if n := len(rec.requests()); n != 0 {
					t.Errorf("an empty key sent %d requests", n)
				}

				llm, err := fetch("refused-credential-reference")
				if err != nil {
					t.Fatalf("placeholder key: %v", err)
				}
				_, _ = llms.GenerateFromSinglePrompt(context.Background(), llm, "hi")
				reqs := rec.requests()
				if len(reqs) == 0 {
					t.Fatal("no request reached the configured upstream")
				}
				for _, req := range reqs {
					if !strings.Contains(req, "refused-credential-reference") {
						t.Errorf("request did not carry the configured key:\n%s", req)
					}
				}
				assertNoCanary(t, reqs)
			})
		}
	}

	for _, vendor := range []models.Vendor{models.GOOGLEAI, models.HUGGINGFACE} {
		t.Run(string(vendor)+"/empty-key", func(t *testing.T) {
			conf := &models.LLM{Vendor: vendor, APIKey: ""}
			if _, err := FetchDriver(conf, &models.LLMSettings{ModelName: "m"}, nil, nil); err == nil {
				t.Errorf("an empty key built a driver")
			}
		})
	}
	t.Run(string(models.GOOGLEAI)+"/internal-routing/empty-key", func(t *testing.T) {
		conf := &models.LLM{Vendor: models.GOOGLEAI, APIKey: ""}
		if _, err := FetchDriver(conf, nil, nil, nil, WithHTTPClient(http.DefaultClient)); err == nil {
			t.Errorf("an empty key built a driver")
		}
	})

	t.Run(string(models.BEDROCK)+"/empty-key", func(t *testing.T) {
		conf := &models.LLM{Vendor: models.BEDROCK, APIEndpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
			Metadata: models.JSONMap{"aws_access_key_id": "", "aws_secret_access_key": ""}}
		if _, err := bedrockVendor.NewBedrockClient(conf); err == nil {
			t.Errorf("empty AWS keys built a client")
		}
	})
}

// The same holds for embedders: an empty key is refused, not filled from the
// host environment.
func TestEmbeddersNeverFallBackToEnvironment(t *testing.T) {
	setEnvCanaries(t)

	for _, vendor := range []models.Vendor{models.OPENAI, models.GOOGLEAI, models.HUGGINGFACE} {
		t.Run(string(vendor), func(t *testing.T) {
			rec := &recorder{}
			srv := httptest.NewServer(rec)
			defer srv.Close()

			spec := &models.EmbedderSpec{Vendor: vendor, Endpoint: srv.URL + "/v1", Model: "m"}
			if e, err := GetEmbedder(spec); err == nil {
				_, _ = e.EmbedQuery(context.Background(), "hi")
				t.Errorf("an empty key built an embedder")
			}
			assertNoCanary(t, rec.requests())
		})
	}
}
