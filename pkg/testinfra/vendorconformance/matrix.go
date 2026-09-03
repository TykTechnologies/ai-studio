package vendorconformance

import (
	"fmt"
	"sort"
	"strings"
)

// Support states what a (vendor, surface) or (vendor, scenario) combination is
// expected to do. It exists so that "this vendor cannot do tool calls" is a
// recorded, reviewable claim rather than a test nobody noticed was missing.
type Support int

const (
	// Unsupported means the platform genuinely cannot serve this combination.
	// The test is skipped with the reason printed.
	Unsupported Support = iota
	// Supported means the combination must work. A failure is a real failure.
	Supported
	// ModelDependent means support varies by the model configured, so a failure
	// is reported as a warning rather than a test failure. Used for self-hosted
	// and aggregator vendors where we cannot know the model's capabilities.
	ModelDependent
)

func (s Support) String() string {
	switch s {
	case Supported:
		return "supported"
	case ModelDependent:
		return "model-dependent"
	default:
		return "unsupported"
	}
}

// vendorMatrix is the declared capability table from
// features/VendorConformance.md §1.2. It is a hypothesis, not gospel: the
// harness reports an "unexpected support" line whenever an Unsupported
// combination actually succeeds, which is how the table stays honest.
type vendorMatrix struct {
	surfaces  map[Surface]Support
	scenarios map[Scenario]Support
}

var matrix = map[string]vendorMatrix{
	"openai": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			SurfaceSDK: Supported, SurfaceUniversal: Supported,
		},
		scenarios: allScenariosSupported(nil),
	},
	"anthropic": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			SurfaceSDK: Supported, SurfaceUniversal: Supported,
		},
		// Anthropic has no OpenAI-style response_format; structured output is
		// achieved through tool use, which the tools scenario already covers.
		scenarios: allScenariosSupported(map[Scenario]Support{
			ScenarioJSONMode: Unsupported,
		}),
	},
	"google_ai": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			SurfaceSDK: Supported, SurfaceUniversal: Supported,
		},
		scenarios: allScenariosSupported(nil),
	},
	"vertex": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			// APIEndpoint is "project:location", not a URL, so /llm/call/ has
			// nothing to proxy to. See vendors/vertex/vertex.go:100.
			SurfaceSDK: Unsupported, SurfaceUniversal: Supported,
		},
		scenarios: allScenariosSupported(nil),
	},
	"bedrock": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			SurfaceSDK: Supported, SurfaceUniversal: Supported,
		},
		scenarios: allScenariosSupported(map[Scenario]Support{
			ScenarioJSONMode: Unsupported,
		}),
	},
	"ollama": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			SurfaceSDK: Supported, SurfaceUniversal: Supported,
		},
		scenarios: allScenariosSupported(map[Scenario]Support{
			ScenarioTools:          ModelDependent,
			ScenarioToolsStreaming: ModelDependent,
			ScenarioVision:         ModelDependent,
			ScenarioLongContext:    ModelDependent,
		}),
	},
	"huggingface": {
		surfaces: map[Surface]Support{
			SurfaceChat: Supported, SurfaceShim: Supported,
			SurfaceSDK: Supported, SurfaceUniversal: Supported,
		},
		scenarios: allScenariosSupported(map[Scenario]Support{
			ScenarioTools:          ModelDependent,
			ScenarioToolsStreaming: ModelDependent,
			ScenarioJSONMode:       Unsupported,
			ScenarioVision:         Unsupported,
			ScenarioLongContext:    ModelDependent,
		}),
	},
}

// compatMatrix is used for every VT_COMPAT_n_* vendor. They speak the OpenAI
// wire protocol but their capabilities are unknowable, so everything beyond the
// basics is model-dependent.
var compatMatrix = vendorMatrix{
	surfaces: map[Surface]Support{
		SurfaceChat: Supported, SurfaceShim: Supported,
		SurfaceSDK: Supported, SurfaceUniversal: Supported,
	},
	scenarios: allScenariosSupported(map[Scenario]Support{
		ScenarioTools:          ModelDependent,
		ScenarioToolsStreaming: ModelDependent,
		ScenarioJSONMode:       ModelDependent,
		ScenarioVision:         ModelDependent,
		ScenarioLongContext:    ModelDependent,
	}),
}

func allScenariosSupported(overrides map[Scenario]Support) map[Scenario]Support {
	out := map[Scenario]Support{}
	for _, s := range AllScenarios {
		out[s] = Supported
	}
	for s, v := range overrides {
		out[s] = v
	}
	return out
}

// unsupportedFeatures are scenarios the PLATFORM does not implement yet, for any
// vendor. They override the per-vendor table, which describes what each vendor
// is capable of — a different question from what we have built.
//
// Kept as a list rather than deleted so the scenario, its request builder and
// its assertions stay alive and reviewable. Removing an entry here is the whole
// of the work needed to start testing a feature the day support lands; a deleted
// scenario would have to be rediscovered and rewritten.
var unsupportedFeatures = map[Scenario]string{
	ScenarioVision: "multimodal input is not officially supported yet; " +
		"remove this entry from unsupportedFeatures when it is",
}

func matrixFor(vendorKey string) vendorMatrix {
	if m, ok := matrix[vendorKey]; ok {
		return m
	}
	return compatMatrix
}

// UnsupportedFeatureReason returns why a scenario is switched off platform-wide,
// or "" if it is not.
func UnsupportedFeatureReason(s Scenario) string {
	return unsupportedFeatures[s]
}

// SurfaceSupport reports whether a vendor is expected to serve a surface.
func SurfaceSupport(vendorKey string, s Surface) Support {
	return matrixFor(vendorKey).surfaces[s]
}

// ScenarioSupport reports whether a vendor is expected to serve a scenario.
// A platform-wide gap wins over whatever the vendor is capable of.
func ScenarioSupport(vendorKey string, s Scenario) Support {
	if _, off := unsupportedFeatures[s]; off {
		return Unsupported
	}
	return matrixFor(vendorKey).scenarios[s]
}

// Case is one leaf of the test matrix: everything needed to build, send and
// judge a single request.
type Case struct {
	Vendor    VendorConfig
	Surface   Surface
	Scenario  Scenario
	ModelSlot ModelSlot
	Model     string
	// Support is the weaker of the surface's and the scenario's declared
	// support, since a case is only as supported as its least-supported axis.
	Support Support
}

// Name is the subtest name, stable and filterable with `go test -run`.
func (c Case) Name() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.Vendor.Key, c.Surface, c.ModelSlot, c.Scenario)
}

// Cases expands the configured vendors, surfaces, scenarios and model slots
// into every leaf case, including Unsupported ones. Callers decide whether to
// skip an unsupported case or probe it; the expansion itself hides nothing.
//
// Pass surfaces to restrict the expansion to the surfaces a particular harness
// can serve — the microgateway harness cannot run SurfaceChat, and the Studio
// harness runs only SurfaceChat.
func (c *Config) Cases(surfaces ...Surface) []Case {
	want := c.Surfaces
	if len(surfaces) > 0 {
		want = nil
		for _, s := range surfaces {
			if c.HasSurface(s) {
				want = append(want, s)
			}
		}
	}

	var out []Case
	for _, v := range c.Vendors {
		for _, surface := range want {
			surfaceSupport := SurfaceSupport(v.Key, surface)
			for _, slot := range v.ModelSlots(c.IncludePrevModel) {
				model, _ := v.Model(slot)
				for _, scenario := range c.Scenarios {
					out = append(out, Case{
						Vendor:    v,
						Surface:   surface,
						Scenario:  scenario,
						ModelSlot: slot,
						Model:     model,
						Support:   weakest(surfaceSupport, ScenarioSupport(v.Key, scenario)),
					})
				}
			}
		}
	}
	return out
}

// weakest returns the more restrictive of two support levels.
func weakest(a, b Support) Support {
	if a == Unsupported || b == Unsupported {
		return Unsupported
	}
	if a == ModelDependent || b == ModelDependent {
		return ModelDependent
	}
	return Supported
}

// Summary renders the human-readable run summary. Every configured vendor gets
// a line and every skipped vendor gets its reason, so a run that silently tested
// nothing is impossible to mistake for a clean run.
func (c *Config) Summary() string {
	var b strings.Builder
	b.WriteString("=== VENDOR CONFORMANCE CONFIG ===\n")

	if len(c.Vendors) == 0 {
		b.WriteString("  (no vendors configured)\n")
	}
	for _, v := range c.Vendors {
		slots := v.ModelSlots(c.IncludePrevModel)
		names := make([]string, 0, len(slots))
		for _, s := range slots {
			m, _ := v.Model(s)
			names = append(names, fmt.Sprintf("%s=%s", s, m))
		}

		var surfaces []string
		for _, s := range c.Surfaces {
			switch SurfaceSupport(v.Key, s) {
			case Supported:
				surfaces = append(surfaces, string(s))
			case ModelDependent:
				surfaces = append(surfaces, string(s)+"?")
			default:
				surfaces = append(surfaces, string(s)+"(n/a)")
			}
		}

		fmt.Fprintf(&b, "  %-14s %-12s %s\n", v.Key, string(v.Vendor), strings.Join(names, " "))
		fmt.Fprintf(&b, "  %-14s surfaces: %s\n", "", strings.Join(surfaces, " "))

		var unsupported []string
		for _, s := range c.Scenarios {
			if ScenarioSupport(v.Key, s) == Unsupported {
				unsupported = append(unsupported, string(s))
			}
		}
		if len(unsupported) > 0 {
			sort.Strings(unsupported)
			fmt.Fprintf(&b, "  %-14s n/a: %s\n", "", strings.Join(unsupported, " "))
		}
	}

	for _, s := range c.Skipped {
		fmt.Fprintf(&b, "  %-14s SKIPPED - %s\n", s.Key, s.Reason)
	}

	fmt.Fprintf(&b, "  ---\n  surfaces=%s\n  scenarios=%d  max_tokens=%d  timeout=%s  strict_unknown=%v\n",
		joinSurfaces(c.Surfaces), len(c.Scenarios), c.MaxTokens, c.Timeout, c.StrictUnknownFields)
	fmt.Fprintf(&b, "  artifacts=%s\n", c.ArtifactDir)
	return b.String()
}
