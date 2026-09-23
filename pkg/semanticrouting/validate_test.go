package semanticrouting

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() Config {
	return Config{
		Slug: "smart",
		Settings: Settings{
			Embedding:    &ModelRef{LLMID: 9, Model: "embed"},
			DefaultRoute: "simple",
		},
		Routes: []Route{
			{Name: "complex", Utterances: []string{"prove it"}, Target: Target{Type: TargetLLM, LLMID: 1, Model: "opus"}},
			{Name: "simple", Target: Target{Type: TargetModelRouter, ModelRouterID: 2, Model: "cheap"}},
		},
	}
}

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(validConfig()))

	cases := map[string]struct {
		mutate func(c *Config)
		field  string
	}{
		"no routes":                  {func(c *Config) { c.Routes = nil }, "routes"},
		"bad route name":             {func(c *Config) { c.Routes[0].Name = "Complex Route" }, "routes[0].name"},
		"reserved route name":        {func(c *Config) { c.Routes[0].Name = "auto" }, "routes[0].name"},
		"duplicate route":            {func(c *Config) { c.Routes[1].Name = "complex"; c.Settings.DefaultRoute = "complex" }, "routes[1].name"},
		"threshold above one":        {func(c *Config) { c.Routes[0].Threshold = 1.5 }, "routes[0].threshold"},
		"bad regex":                  {func(c *Config) { c.Routes[0].Keywords = []Keyword{{Pattern: "(", Regex: true}} }, "routes[0].keywords[0]"},
		"empty keyword":              {func(c *Config) { c.Routes[0].Keywords = []Keyword{{Pattern: "  "}} }, "routes[0].keywords[0]"},
		"empty utterance":            {func(c *Config) { c.Routes[0].Utterances = []string{""} }, "routes[0].utterances[0]"},
		"llm target without id":      {func(c *Config) { c.Routes[0].Target.LLMID = 0 }, "routes[0].target.llm_id"},
		"router target without id":   {func(c *Config) { c.Routes[1].Target.ModelRouterID = 0 }, "routes[1].target.model_router_id"},
		"unknown target type":        {func(c *Config) { c.Routes[0].Target.Type = "tool" }, "routes[0].target.type"},
		"target without model":       {func(c *Config) { c.Routes[0].Target.Model = "" }, "routes[0].target.model"},
		"no default route":           {func(c *Config) { c.Settings.DefaultRoute = "" }, "settings.default_route"},
		"unknown default route":      {func(c *Config) { c.Settings.DefaultRoute = "nope" }, "settings.default_route"},
		"examples without embedding": {func(c *Config) { c.Settings.Embedding = nil }, "settings.embedding"},
		"judge without model":        {func(c *Config) { c.Settings.Judge = JudgeSettings{Enabled: true} }, "settings.judge"},
		"bad judge when": {func(c *Config) {
			c.Settings.Judge = JudgeSettings{Enabled: true, ModelRef: ModelRef{LLMID: 1, Model: "m"}, When: "sometimes"}
		}, "settings.judge.when"},
		"bad mode":  {func(c *Config) { c.Settings.Mode = "dry" }, "settings.mode"},
		"bad scope": {func(c *Config) { c.Settings.InputScope = "everything" }, "settings.input_scope"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			tc.mutate(&c)
			err := Validate(c)
			var verr *ValidationError
			require.True(t, errors.As(err, &verr), "want a ValidationError, got %v", err)
			assert.Equal(t, tc.field, verr.Field)
		})
	}
}

func TestValidate_RoutesWithoutExamplesNeedNoEmbedding(t *testing.T) {
	c := validConfig()
	c.Routes[0].Utterances = nil
	c.Routes[0].Keywords = []Keyword{{Pattern: "prove"}}
	c.Settings.Embedding = nil
	assert.NoError(t, Validate(c))
}

func TestModelsFor(t *testing.T) {
	c := validConfig()
	assert.Equal(t, []string{"smart/auto"}, ModelsFor("smart", c))
	c.Settings.AllowExplicitRoute = true
	assert.Equal(t, []string{"smart/auto", "smart/complex", "smart/simple"}, ModelsFor("smart", c))
}

func TestEffectiveMode(t *testing.T) {
	assert.Equal(t, ModeEnforce, Settings{}.EffectiveMode())
	assert.Equal(t, ModeShadow, Settings{Mode: ModeShadow}.EffectiveMode())
}
