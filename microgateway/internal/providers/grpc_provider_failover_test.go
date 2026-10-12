package providers

import (
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// The in-memory path must carry the native API resource signing keys too, or
// those edges would sign resource IDs with the vendor-credential key.
func TestConvertPBLLMToDatabase_ResourceSigningKeys(t *testing.T) {
	p := &GRPCProvider{}
	expires := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	with := p.convertPBLLMToDatabase(&pb.LLMConfig{Id: 1, Name: "claude",
		ResourceSigningSecretEncrypted:     "enc-current",
		ResourceSigningSecretPrevEncrypted: "enc-prev",
		ResourceSigningPrevExpiresAt:       timestamppb.New(expires),
		VendorWorkspaceId:                  "wrkspc_pinned",
		PriceMultipliers:                   `{"batch":0.4}`})
	assert.JSONEq(t, `{"batch":0.4}`, string(with.PriceMultipliers))
	assert.Equal(t, "wrkspc_pinned", with.VendorWorkspaceID)
	assert.Equal(t, "enc-current", with.ResourceSigningSecretEncrypted)
	assert.Equal(t, "enc-prev", with.ResourceSigningSecretPrevEncrypted)
	if assert.NotNil(t, with.ResourceSigningPrevExpiresAt) {
		assert.True(t, with.ResourceSigningPrevExpiresAt.Equal(expires))
	}

	without := p.convertPBLLMToDatabase(&pb.LLMConfig{Id: 2, Name: "plain"})
	assert.Empty(t, without.ResourceSigningSecretEncrypted)
	assert.Nil(t, without.ResourceSigningPrevExpiresAt)
	assert.Nil(t, without.PriceMultipliers)
}
