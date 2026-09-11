package providers

import (
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
)

// The in-memory hub-spoke path bypasses SQLite, so the converter must carry
// the failover waterfall itself or edges on that path would never fail over.
func TestConvertPBLLMToDatabase_Failover(t *testing.T) {
	p := &GRPCProvider{}

	waterfall := `{"targets":[{"llm_id":2,"model":"gpt-4o"}]}`
	with := p.convertPBLLMToDatabase(&pb.LLMConfig{Id: 1, Name: "gpt", Failover: waterfall})
	assert.JSONEq(t, waterfall, string(with.Failover))

	without := p.convertPBLLMToDatabase(&pb.LLMConfig{Id: 2, Name: "plain"})
	assert.Nil(t, without.Failover, "an empty string must stay NULL, not become an empty JSON value")
}
