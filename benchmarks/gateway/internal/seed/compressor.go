package seed

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The Prompt Compressor scenario (eac.15, s8-prompt-compressor.yaml)
// measures the time the community plugin adds per request. seed
// -compressor <command> adds one LLM per configuration, all on the instant
// mock, and three plugin instances, because mode is a plugin-wide setting:
//
//	mock-pc-none      no plugin (the gateway alone)
//	mock-pc-attached  plugin attached, mode off
//	mock-pc-audit     mode audit, level 1
//	mock-pc-l1..l3    mode enforce, levels 1, 2 and 3 (level 3 with masking)
//
// The plugin binary must be at the command's path in both the Studio and the
// edge container (compose/prompt-compressor.yaml mounts it).
var CompressorLLMs = []string{"mock-pc-none", "mock-pc-attached", "mock-pc-audit", "mock-pc-l1", "mock-pc-l2", "mock-pc-l3"}

// compressorPlugins are the plugin instances and the LLMs each is attached
// to. Policies apply to every tool output (min_unit_tokens 0) so the
// benchmark measures the work, not the selection.
func compressorPlugins() []struct {
	name   string
	config map[string]any
	llms   []string
} {
	policy := func(level int) map[string]any {
		return map[string]any{"level": level, "min_unit_tokens": 0}
	}
	l3 := policy(3)
	l3["mask_after_turns"] = 2
	l3["mask_step_tokens"] = 1000
	return []struct {
		name   string
		config map[string]any
		llms   []string
	}{
		{"pc-bench-off", map[string]any{"mode": "off"}, []string{"mock-pc-attached"}},
		{"pc-bench-audit", map[string]any{"mode": "audit", "default_policy": "l1", "policies": map[string]any{"l1": policy(1)}}, []string{"mock-pc-audit"}},
		{"pc-bench-enforce", map[string]any{
			"mode": "enforce", "default_policy": "l1",
			"policies": map[string]any{"l1": policy(1), "l2": policy(2), "l3": l3},
			"bindings": map[string]any{"llm:mock-pc-l1": "l1", "llm:mock-pc-l2": "l2", "llm:mock-pc-l3": "l3"},
			// The mock LLMs have no request filters; say so, so level 3 is
			// not capped while the plugin learns that.
			"lossy_with_request_filters": true,
		}, []string{"mock-pc-l1", "mock-pc-l2", "mock-pc-l3"}},
	}
}

// compressorLLMSpecs are the LLM rows the scenario needs.
func compressorLLMSpecs(mock string) []LLMSpec {
	var out []LLMSpec
	for _, name := range CompressorLLMs {
		out = append(out, LLMSpec{Name: name, Vendor: "openai", Endpoint: mock + "/p/" + url.PathEscape(MockProfiles["fast"]) + "/v1",
			APIKey: "mock-key", DefaultModel: MockModel})
	}
	return out
}

type pluginAttrs struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// EnsurePlugin registers a plugin (or updates the one with the same name),
// loads it and approves its service scopes, and returns its id.
func (s *Studio) EnsurePlugin(ctx context.Context, name, command string, config map[string]any) (int, error) {
	var list jsonAPIList[pluginAttrs]
	if err := s.mustOK(ctx, http.MethodGet, "/api/v1/plugins?limit=100", nil, &list, http.StatusOK); err != nil {
		return 0, err
	}
	id := ""
	for _, d := range list.Data {
		if d.Attributes.Name == name {
			id = d.ID
		}
	}
	if id == "" {
		var created jsonAPI[pluginAttrs]
		body := map[string]any{"name": name, "description": "gateway benchmark", "command": command,
			"is_active": true, "load_immediately": true, "config": config}
		if err := s.mustOK(ctx, http.MethodPost, "/api/v1/plugins", body, &created, http.StatusCreated, http.StatusOK); err != nil {
			return 0, err
		}
		id = created.Data.ID
	} else {
		body := map[string]any{"command": command, "config": config, "is_active": true}
		if err := s.mustOK(ctx, http.MethodPatch, "/api/v1/plugins/"+id, body, nil, http.StatusOK); err != nil {
			return 0, err
		}
	}
	if err := s.mustOK(ctx, http.MethodPost, "/api/v1/plugins/"+id+"/validate-and-load", map[string]any{"command": command}, nil, http.StatusOK); err != nil {
		return 0, err
	}
	if err := s.mustOK(ctx, http.MethodPost, "/api/v1/plugins/"+id+"/approve-scopes", map[string]any{"approved": true}, nil, http.StatusOK); err != nil {
		return 0, err
	}
	if err := s.mustOK(ctx, http.MethodPost, "/api/v1/plugins/"+id+"/reload", map[string]any{}, nil, http.StatusOK); err != nil {
		return 0, err
	}
	return strconv.Atoi(id)
}

// AttachPlugins sets an LLM's plugins, in order.
func (s *Studio) AttachPlugins(ctx context.Context, llmID int, pluginIDs []int) error {
	return s.mustOK(ctx, http.MethodPut, "/api/v1/llms/"+strconv.Itoa(llmID)+"/plugins", map[string]any{"plugin_ids": pluginIDs}, nil, http.StatusOK)
}

// seedCompressor creates the plugin instances and attaches them.
func seedCompressor(ctx context.Context, s *Studio, command string, llmIDs map[string]int, log func(string, ...any)) error {
	for _, p := range compressorPlugins() {
		id, err := s.EnsurePlugin(ctx, p.name, command, p.config)
		if err != nil {
			return fmt.Errorf("plugin %s: %w", p.name, err)
		}
		for _, llm := range p.llms {
			if err := s.AttachPlugins(ctx, llmIDs[llm], []int{id}); err != nil {
				return fmt.Errorf("attaching %s to %s: %w", p.name, llm, err)
			}
		}
		log("plugin %-16s -> %s", p.name, strings.Join(p.llms, ", "))
	}
	// The plugin's first session can start before its scopes are approved;
	// give the reload a moment before the config push
	time.Sleep(3 * time.Second)
	return nil
}
