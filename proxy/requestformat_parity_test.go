package proxy

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestformat"
	"github.com/TykTechnologies/midsommar/v2/switches"
)

// nativeChainHookFor returns the chain hook that applies to r on llm, or nil.
func nativeChainHookFor(r *http.Request, llm *models.LLM) *nativeapi.ChainHook {
	_, _, _, _, _, hook := resolveChainHook(r, llm)
	return hook
}

// inlineRequestFilterFormat is requestFilterFormatFor as it was before
// requestformat.For: the vendor, then the decision format, then the native
// chain hook's format, read from the vendors' own specs.
func inlineRequestFilterFormat(r *http.Request, llm *models.LLM) string {
	format := string(llm.Vendor)
	if df := decisionFilterFormat(r.URL.Path); df != "" {
		format = df
	}
	if hook := nativeChainHookFor(r, llm); hook != nil && hook.FilterFormat != "" {
		format = hook.FilterFormat
	}
	return format
}

// parityPaths are vendor-canonical paths to resolve: every native operation
// and chain hook any vendor declares, the chat paths, and decision paths.
func parityPaths() []string {
	set := map[string]bool{
		"/v1/chat/completions": true, "/chat/completions": true, "/v1/messages": true,
		"/v1/decisions": true, "/v1/systemone": true, "/systemone": true,
		"/v1/models": true, "/v1beta/models/gemini-pro:generateContent": true,
		"/model/anthropic.claude/invoke": true, "/model/x/converse-stream": true,
		"/api/chat": true, "/v1/responses/": true, "/v1/embeddings/extra": true,
	}
	for vendor := range switches.VendorMap {
		nv := nativeVendorFor(vendor)
		if nv == nil {
			continue
		}
		spec := nv.matcher.Spec()
		for _, op := range spec.Operations {
			set[strings.ReplaceAll(strings.ReplaceAll(op.Path, "{id}", "res_1"), "{", "x")] = true
		}
		if spec.ChainHooks != nil {
			for _, h := range spec.ChainHooks.Hooks {
				set[h.Path] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// The proxy's format resolution moved into requestformat.For. Every vendor ×
// endpoint shape × mode × method × path agrees with the inline logic it
// replaced, which read the vendors' native chain hooks directly.
func TestRequestFormatParity(t *testing.T) {
	endpoints := []string{
		"", "https://api.openai.com", "https://api.openai.com/v1", "https://api.openai.com/v1/",
		"https://gw.example/openai", "https://gw.example/openai/v1", "https://gw.example/anthropic/v1",
		"https://generativelanguage.googleapis.com/v1beta", "http://localhost:11434",
		"https://jev.systemone.example/v1",
	}
	methods := []string{http.MethodPost, http.MethodGet, http.MethodDelete}
	modes := []string{"rest", "stream", "call"}

	var vendors []models.Vendor
	for v := range switches.VendorMap {
		vendors = append(vendors, v)
	}
	sort.Slice(vendors, func(i, j int) bool { return vendors[i] < vendors[j] })

	paths := parityPaths()
	checked, nonVendor := 0, 0
	for _, vendor := range vendors {
		for _, endpoint := range endpoints {
			llm := &models.LLM{Vendor: vendor, APIEndpoint: endpoint}
			for _, mode := range modes {
				for _, canonical := range paths {
					// The caller's path with and without the version segment the
					// endpoint may already carry.
					rests := []string{canonical}
					if seg, tail, ok := strings.Cut(strings.TrimPrefix(canonical, "/"), "/"); ok && requestformat.IsVersionSegment(seg) {
						rests = append(rests, "/"+tail)
					}
					for _, rest := range rests {
						for _, method := range methods {
							r := httptest.NewRequest(method, "/llm/"+mode+"/my-llm"+rest, nil)
							want := inlineRequestFilterFormat(r, llm)
							got := requestFilterFormatFor(r, llm)
							if got != want {
								t.Errorf("%s %s on %s LLM (endpoint %q): requestformat.For = %q, inline = %q",
									method, r.URL.Path, vendor, endpoint, got, want)
							}
							if got != string(vendor) {
								nonVendor++
							}
							checked++
						}
					}
				}
			}
		}
	}
	// The table must exercise the formats it is about, not only vendor names.
	if nonVendor == 0 {
		t.Fatal("no case resolved to a non-vendor format")
	}
	t.Logf("%d cases, %d with a format other than the vendor's", checked, nonVendor)

	// Paths outside the native pass-through keep the vendor format but for a
	// decision suffix, as before.
	for _, p := range []string{"/ai/my-llm/v1/chat/completions", "/anthropic/my-llm/v1/messages", "/v1/responses", "/llm/bogus/my-llm/v1/responses"} {
		r := httptest.NewRequest(http.MethodPost, p, nil)
		llm := &models.LLM{Vendor: models.OPENAI}
		if got, want := requestFilterFormatFor(r, llm), inlineRequestFilterFormat(r, llm); got != want {
			t.Errorf("POST %s: requestformat.For = %q, inline = %q", p, got, want)
		}
	}
}

// requestformat.ChainFormats and the vendors' chain hooks name the same
// operations with the same formats, so the table cannot drift from the specs.
func TestRequestFormatChainFormatsMatchVendorSpecs(t *testing.T) {
	type key struct{ vendor, method, path string }
	fromSpecs := map[key]string{}
	for vendor := range switches.VendorMap {
		nv := nativeVendorFor(vendor)
		if nv == nil || nv.matcher.Spec().ChainHooks == nil {
			continue
		}
		for _, h := range nv.matcher.Spec().ChainHooks.Hooks {
			if h.FilterFormat != "" {
				fromSpecs[key{string(vendor), h.Method, h.Path}] = h.FilterFormat
			}
		}
	}
	fromTable := map[key]string{}
	for vendor, ops := range requestformat.ChainFormats {
		for _, op := range ops {
			fromTable[key{vendor, op.Method, op.Path}] = op.Format
		}
	}
	for k, f := range fromSpecs {
		if fromTable[k] != f {
			t.Errorf("%s %s %s: vendor spec says %q, requestformat.ChainFormats %q", k.vendor, k.method, k.path, f, fromTable[k])
		}
	}
	for k, f := range fromTable {
		if fromSpecs[k] != f {
			t.Errorf("%s %s %s: requestformat.ChainFormats says %q, vendor spec %q", k.vendor, k.method, k.path, f, fromSpecs[k])
		}
	}
	if len(fromTable) == 0 {
		t.Fatal("requestformat.ChainFormats is empty")
	}
}
