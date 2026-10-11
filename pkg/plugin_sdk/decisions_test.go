package plugin_sdk

import (
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDecision_EitherWireFormatReadsAsOpenAI(t *testing.T) {
	systemOne := `{"model":"jev-latest","state":"charged twice",` +
		`"questions":{"angry":{"type":"noul","instructions":"Angry?"},"topic":{"type":"choice","criteria":{"shipping":"","billing":""}}}}`
	req, err := ParseDecisionRequest("/llm/rest/jev/v1/systemone", []byte(systemOne))
	require.NoError(t, err)
	require.Len(t, req.Questions, 2)
	assert.Equal(t, DecisionPredicate, req.Questions[0].Type)
	assert.Equal(t, "billing", req.Questions[1].Choices[1].Value)

	resp, err := ParseDecisionResponse([]byte(`{"answers":{"angry":{"type":"noul","noul":0.9}}}`), req)
	require.NoError(t, err)
	require.Len(t, resp.Answers, 2)
	assert.Equal(t, DecisionPredicate, resp.Answers[0].Type)
	assert.Equal(t, DecisionRefusal, resp.Answers[1].Type, "an unanswered question is a refusal")
	assert.Equal(t, "angry: predicate p=0.9\ntopic: refused", DecisionAnswerText(resp))

	openAI := `{"model":"gpt-6-luna","input":"x","questions":[{"type":"score","name":"u","levels":[{"label":"lo"},{"label":"hi"}]}]}`
	req, err = ParseDecisionRequest("/v1/decisions", []byte(openAI))
	require.NoError(t, err)
	assert.Equal(t, DecisionScore, req.Questions[0].Type)
	assert.True(t, IsDecisionPath("/v1/decisions"))
	assert.False(t, IsDecisionPath("/v1/chat/completions"))
}

func TestPluginContext_CarriesTheRequestKind(t *testing.T) {
	w := newPluginServerWrapper(nil, RuntimeGateway, nil)
	ctx := w.createPluginContext(t.Context(), &pb.PluginContext{RequestKind: RequestKindDecision, Vendor: "systemone"})
	assert.True(t, ctx.IsDecision())
	assert.Equal(t, "systemone", ctx.Vendor)

	ctx = w.createPluginContext(t.Context(), &pb.PluginContext{})
	assert.False(t, ctx.IsDecision())
}
