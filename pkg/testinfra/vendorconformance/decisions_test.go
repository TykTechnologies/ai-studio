package vendorconformance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateDecisionEnv points the loader at an empty env file and clears every
// variable it reads, so the developer's real vendors.env cannot leak in.
func isolateDecisionEnv(t *testing.T) {
	t.Helper()
	empty := t.TempDir() + "/empty.env"
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	t.Setenv("VENDOR_TESTS_ENV_FILE", empty)
	for _, k := range []string{
		"VENDOR_TESTS_ENABLED", "VENDOR_TESTS_VENDORS", "VENDOR_TESTS_DECISION_SCENARIOS", "VENDOR_TESTS_ARTIFACT_DIR",
		"VT_OPENAI_API_KEY", "VT_OPENAI_ENDPOINT", "VT_OPENAI_MODEL_DECISION",
		"VT_SYSTEMONE_MODEL", "VT_SYSTEMONE_ENDPOINT", "VT_SYSTEMONE_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadDecisions(t *testing.T) {
	t.Run("nothing configured skips both targets with reasons", func(t *testing.T) {
		isolateDecisionEnv(t)
		cfg, err := LoadDecisions()
		require.NoError(t, err)
		assert.Empty(t, cfg.Targets)
		require.Len(t, cfg.Skipped, 2)
		assert.Equal(t, "openai", cfg.Skipped[0].Key)
		assert.Contains(t, cfg.Skipped[0].Reason, "VT_OPENAI_API_KEY")
		assert.Contains(t, cfg.Skipped[1].Reason, "VT_SYSTEMONE_MODEL")
		assert.Equal(t, AllDecisionScenarios, cfg.Scenarios)
	})

	t.Run("openai reuses the chat key and needs a decision model", func(t *testing.T) {
		isolateDecisionEnv(t)
		t.Setenv("VT_OPENAI_API_KEY", "sk-test")
		cfg, err := LoadDecisions()
		require.NoError(t, err)
		assert.Empty(t, cfg.Targets)
		assert.Contains(t, cfg.Skipped[0].Reason, "VT_OPENAI_MODEL_DECISION")

		t.Setenv("VT_OPENAI_MODEL_DECISION", "gpt-6-luna")
		cfg, err = LoadDecisions()
		require.NoError(t, err)
		target, ok := cfg.Target("openai")
		require.True(t, ok)
		assert.Equal(t, models.OPENAI, target.Vendor)
		assert.Equal(t, "https://api.openai.com/v1", target.Endpoint)
		assert.Equal(t, "vt-dec-openai", target.Slug())
		assert.Equal(t, "decisions", target.NativePath(), "the endpoint already carries /v1")
		assert.Equal(t, FormatOpenAIDecision, target.NativeFormat())
	})

	t.Run("systemone hosted needs a key, a self-host does not", func(t *testing.T) {
		isolateDecisionEnv(t)
		t.Setenv("VT_SYSTEMONE_MODEL", "jev-latest")
		cfg, err := LoadDecisions()
		require.NoError(t, err)
		_, ok := cfg.Target("systemone")
		assert.False(t, ok)
		assert.Contains(t, cfg.Skipped[len(cfg.Skipped)-1].Reason, "VT_SYSTEMONE_API_KEY")

		t.Setenv("VT_SYSTEMONE_ENDPOINT", "http://localhost:8080")
		cfg, err = LoadDecisions()
		require.NoError(t, err)
		target, ok := cfg.Target("systemone")
		require.True(t, ok)
		assert.Equal(t, models.SYSTEMONE, target.Vendor)
		assert.Equal(t, "v1/systemone", target.NativePath())
		assert.Equal(t, FormatSystemOneDecision, target.NativeFormat())
	})

	t.Run("filters", func(t *testing.T) {
		isolateDecisionEnv(t)
		t.Setenv("VENDOR_TESTS_VENDORS", "systemone")
		t.Setenv("VENDOR_TESTS_DECISION_SCENARIOS", "basic")
		cfg, err := LoadDecisions()
		require.NoError(t, err)
		assert.Equal(t, []DecisionScenario{DecisionBasic}, cfg.Scenarios)
		require.Len(t, cfg.Skipped, 1)
		assert.Equal(t, "systemone", cfg.Skipped[0].Key)

		t.Setenv("VENDOR_TESTS_DECISION_SCENARIOS", "nope")
		_, err = LoadDecisions()
		assert.Error(t, err)
	})
}

func TestDecisionRequests(t *testing.T) {
	for _, scenario := range AllDecisionScenarios {
		req := DecisionRequest(scenario, "m", "ref-1")
		require.NoError(t, decisions.Validate(req, decisions.FormatSystemOne), scenario)
		text, images, err := decisions.InputText(req.Input)
		require.NoError(t, err)
		assert.Zero(t, images)
		assert.Contains(t, text, "charged twice", scenario)
		assert.Contains(t, text, "ref-1", "the ref finds the analytics event")

		body, err := NativeDecisionBody(DecisionTarget{Wire: decisions.FormatSystemOne}, req)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"state"`)
		assert.Less(t, strings.Index(string(body), `"shipping"`), strings.Index(string(body), `"billing"`), "option order survives")
	}
}

func goodOpenAIDecision() string {
	return `{"id":"dec_1","object":"decision","model":"gpt-6-luna","answers":[
	  {"type":"predicate","name":"is_billing","probability":0.97},
	  {"type":"choice","name":"team","choice":"billing","confidence":0.9,
	   "probabilities":[{"value":"shipping","probability":0.02},{"value":"billing","probability":0.95},{"value":"technical","probability":0.03}]},
	  {"type":"score","name":"urgency","score":1.8,"confidence":0.7,
	   "probabilities":[{"value":0,"label":"can wait","probability":0.05},{"value":1,"label":"this week","probability":0.1},{"value":2,"label":"today","probability":0.85}]}],
	  "usage":{"input_tokens":140,"output_tokens":0}}`
}

func TestDecisionSchemas(t *testing.T) {
	res, err := Validate(FormatOpenAIDecision, []byte(goodOpenAIDecision()))
	require.NoError(t, err)
	assert.True(t, res.OK(), res.Error())
	assert.Empty(t, res.Drift)

	// An undocumented field is drift, not a violation.
	res, err = Validate(FormatOpenAIDecision, []byte(strings.Replace(goodOpenAIDecision(), `"object":"decision"`, `"object":"decision","service_tier":"default"`, 1)))
	require.NoError(t, err)
	assert.True(t, res.OK())
	require.Len(t, res.Drift, 1)
	assert.Equal(t, "service_tier", res.Drift[0].Pointer)

	bad := map[string]string{
		"no usage":             `{"answers":[]}`,
		"predicate without p":  `{"answers":[{"type":"predicate","name":"a"}],"usage":{"input_tokens":1}}`,
		"probability above 1":  `{"answers":[{"type":"predicate","name":"a","probability":1.5}],"usage":{"input_tokens":1}}`,
		"unknown answer type":  `{"answers":[{"type":"noul","name":"a"}],"usage":{"input_tokens":1}}`,
		"usage without counts": `{"answers":[],"usage":{}}`,
	}
	for name, body := range bad {
		res, err := Validate(FormatOpenAIDecision, []byte(body))
		require.NoError(t, err, name)
		assert.False(t, res.OK(), name)
	}

	native := `{"model":"jev-1.13.0","answers":{"is_billing":{"type":"noul","noul":0.9},"team":{"type":"choice","choice":"billing","probabilities":{"billing":1}}},"usage":{"input_tokens":200,"output_tokens":0}}`
	res, err = Validate(FormatSystemOneDecision, []byte(native))
	require.NoError(t, err)
	assert.True(t, res.OK(), res.Error())

	res, err = Validate(FormatSystemOneDecision, []byte(`{"answers":{"a":{"type":"noul"}},"usage":{"input_tokens":1}}`))
	require.NoError(t, err)
	assert.False(t, res.OK(), "noul without its probability")
}

func TestCheckDecisionResponse(t *testing.T) {
	req := DecisionRequest(DecisionBasic, "m", "")
	parse := func(body string) *decisions.Response {
		resp, err := decisions.ParseResponse([]byte(body), req)
		require.NoError(t, err)
		return resp
	}

	good := parse(goodOpenAIDecision())
	assert.Empty(t, CheckDecisionResponse(req, good))
	assert.Empty(t, DecisionPlausibility(good))
	env := EnvelopeOf(good)
	assert.Equal(t, []string{"is_billing", "team", "urgency"}, env.Names)
	assert.True(t, env.HasUsage)

	// Refusals are allowed in place of any answer.
	refused := strings.Replace(goodOpenAIDecision(), `{"type":"predicate","name":"is_billing","probability":0.97}`, `{"type":"refusal","name":"is_billing"}`, 1)
	assert.Empty(t, CheckDecisionResponse(req, parse(refused)))

	cases := map[string]struct {
		body string
		want string
	}{
		"out of order":             {strings.Replace(strings.Replace(goodOpenAIDecision(), `"name":"is_billing"`, `"name":"X"`, 1), `"name":"team"`, `"name":"is_billing"`, 1), "question order"},
		"wrong type":               {strings.Replace(goodOpenAIDecision(), `"type":"predicate","name":"is_billing","probability":0.97`, `"type":"score","name":"is_billing","score":1`, 1), "want \"predicate\""},
		"unknown option":           {strings.Replace(goodOpenAIDecision(), `"choice":"billing"`, `"choice":"sales"`, 1), "not one of the options"},
		"score out of range":       {strings.Replace(goodOpenAIDecision(), `"score":1.8`, `"score":4`, 1), "outside [0,2]"},
		"probabilities do not sum": {strings.Replace(goodOpenAIDecision(), `"probability":0.95`, `"probability":0.5`, 1), "sum to"},
		"no input tokens":          {strings.Replace(goodOpenAIDecision(), `"input_tokens":140`, `"input_tokens":0`, 1), "billed by input tokens"},
		"missing answer":           {strings.Replace(goodOpenAIDecision(), `{"type":"predicate","name":"is_billing","probability":0.97},`, ``, 1), "2 answers for 3 questions"},
	}
	for name, tc := range cases {
		problems := CheckDecisionResponse(req, parse(tc.body))
		require.NotEmpty(t, problems, name)
		assert.Contains(t, strings.Join(problems, "\n"), tc.want, name)
	}

	noUsage := parse(`{"answers":[]}`)
	assert.Contains(t, strings.Join(CheckDecisionResponse(req, noUsage), "\n"), "no usage")

	wrong := parse(strings.Replace(goodOpenAIDecision(), `"choice":"billing"`, `"choice":"shipping"`, 1))
	assert.Len(t, DecisionPlausibility(wrong), 1)
}

func TestDecisionEnvelope_TranslatedSystemOneMatchesOpenAI(t *testing.T) {
	req := DecisionRequest(DecisionBasic, "m", "")
	native := `{"model":"jev-1.13.0","answers":{"urgency":{"type":"score","score":1.2,"probabilities":[0.2,0.4,0.4]},` +
		`"team":{"type":"choice","choice":"billing","probabilities":{"technical":0.1,"billing":0.8,"shipping":0.1}},` +
		`"is_billing":{"type":"noul","noul":0.9}},"usage":{"input_tokens":200,"output_tokens":0}}`
	resp, err := decisions.ParseResponse([]byte(native), req)
	require.NoError(t, err)
	assert.Empty(t, CheckDecisionResponse(req, resp))

	openAI, err := decisions.ParseResponse([]byte(goodOpenAIDecision()), req)
	require.NoError(t, err)
	assert.Equal(t, EnvelopeOf(openAI), EnvelopeOf(resp))

	out, err := json.Marshal(resp)
	require.NoError(t, err)
	res, err := Validate(FormatOpenAIDecision, out)
	require.NoError(t, err)
	assert.True(t, res.OK(), "the gateway's translation satisfies the OpenAI schema: %s", res.Error())
}
