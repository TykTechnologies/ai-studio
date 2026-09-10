package providers

import (
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
)

// The hub-spoke in-memory path bypasses SQLite, so the converter must carry
// governed metadata itself or request-path plugins would never see it.
func TestConvertPBLLMToDatabase_GovernedMetadata(t *testing.T) {
	p := &GRPCProvider{}

	with := p.convertPBLLMToDatabase(&pb.LLMConfig{Id: 1, Name: "gpt", GovernedMetadata: `{"data_classification":"restricted"}`})
	assert.JSONEq(t, `{"data_classification":"restricted"}`, string(with.GovernedMetadata))

	without := p.convertPBLLMToDatabase(&pb.LLMConfig{Id: 2, Name: "plain"})
	assert.Nil(t, without.GovernedMetadata)
}
