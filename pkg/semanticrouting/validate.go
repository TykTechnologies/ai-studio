package semanticrouting

import (
	"fmt"
	"regexp"
	"strings"
)

// routeNamePattern is what a route may be called: it is written into model
// strings ("{router}/{route}") and read back from the judge's answer.
var routeNamePattern = regexp.MustCompile(`^[a-z0-9]+([-_][a-z0-9]+)*$`)

// ValidationError is a configuration problem an administrator can fix.
type ValidationError struct {
	Field  string
	Detail string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Detail
	}
	return e.Field + ": " + e.Detail
}

func invalid(field, format string, args ...interface{}) error {
	return &ValidationError{Field: field, Detail: fmt.Sprintf(format, args...)}
}

// Validate checks a router's shape: what can be told without looking up the
// LLMs and Model Routers it names (the hub checks those against its store).
func Validate(cfg Config) error {
	s := cfg.Settings
	if s.Mode != "" && s.Mode != ModeEnforce && s.Mode != ModeShadow {
		return invalid("settings.mode", "must be %q or %q", ModeEnforce, ModeShadow)
	}
	if s.InputScope != "" && s.InputScope != ScopeLastUser && s.InputScope != ScopeAllUser {
		return invalid("settings.input_scope", "must be %q or %q", ScopeLastUser, ScopeAllUser)
	}
	if s.MaxInputChars < 0 {
		return invalid("settings.max_input_chars", "must not be negative")
	}
	if len(cfg.Routes) == 0 {
		return invalid("routes", "a router needs at least one route")
	}

	seen := map[string]bool{}
	needsEmbedding := false
	for i, r := range cfg.Routes {
		field := fmt.Sprintf("routes[%d]", i)
		if !routeNamePattern.MatchString(r.Name) {
			return invalid(field+".name", "%q must be lowercase letters and digits, separated by - or _", r.Name)
		}
		if r.Name == ReservedAutoModel {
			return invalid(field+".name", "%q is reserved (callers send {router}/auto to have the router classify)", r.Name)
		}
		if seen[r.Name] {
			return invalid(field+".name", "duplicate route %q", r.Name)
		}
		seen[r.Name] = true
		if r.Threshold < 0 || r.Threshold > 1 {
			return invalid(field+".threshold", "must be between 0 and 1")
		}
		for j, k := range r.Keywords {
			if strings.TrimSpace(k.Pattern) == "" {
				return invalid(fmt.Sprintf("%s.keywords[%d]", field, j), "must not be empty")
			}
			if k.Regex {
				if _, err := regexp.Compile(k.Pattern); err != nil {
					return invalid(fmt.Sprintf("%s.keywords[%d]", field, j), "invalid regex: %v", err)
				}
			}
		}
		for j, u := range r.Utterances {
			if strings.TrimSpace(u) == "" {
				return invalid(fmt.Sprintf("%s.utterances[%d]", field, j), "must not be empty")
			}
		}
		if len(r.Utterances) > 0 {
			needsEmbedding = true
		}
		switch r.Target.Type {
		case TargetLLM:
			if r.Target.LLMID == 0 {
				return invalid(field+".target.llm_id", "is required")
			}
		case TargetModelRouter:
			if r.Target.ModelRouterID == 0 {
				return invalid(field+".target.model_router_id", "is required")
			}
		default:
			return invalid(field+".target.type", "must be %q or %q", TargetLLM, TargetModelRouter)
		}
		if strings.TrimSpace(r.Target.Model) == "" {
			return invalid(field+".target.model", "is required")
		}
	}

	if s.DefaultRoute == "" {
		return invalid("settings.default_route", "is required: it serves every request nothing else decides")
	}
	if !seen[s.DefaultRoute] {
		return invalid("settings.default_route", "%q is not one of the routes", s.DefaultRoute)
	}
	if needsEmbedding && (s.Embedding == nil || s.Embedding.LLMID == 0 || strings.TrimSpace(s.Embedding.Model) == "") {
		return invalid("settings.embedding", "routes with example utterances need an embedding LLM and model")
	}
	if s.Judge.Enabled {
		if s.Judge.ModelRef.LLMID == 0 || strings.TrimSpace(s.Judge.ModelRef.Model) == "" {
			return invalid("settings.judge", "the judge needs an LLM and model")
		}
		if s.Judge.When != "" && s.Judge.When != JudgeAlways && s.Judge.When != JudgeLowConfidence {
			return invalid("settings.judge.when", "must be %q or %q", JudgeAlways, JudgeLowConfidence)
		}
	}
	if s.Affinity.TTLSeconds < 0 {
		return invalid("settings.affinity.ttl_seconds", "must not be negative")
	}
	return nil
}

// ModelsFor lists the model strings a caller can send for this router:
// "{slug}/auto", and "{slug}/{route}" per route when explicit routes are
// allowed.
func ModelsFor(slug string, cfg Config) []string {
	out := []string{slug + "/" + ReservedAutoModel}
	if cfg.Settings.AllowExplicitRoute {
		for _, r := range cfg.Routes {
			out = append(out, slug+"/"+r.Name)
		}
	}
	return out
}
