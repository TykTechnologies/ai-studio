package services

import (
	"encoding/base64"
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
)

// agentTracingKeyCrypto decrypts the Agent Call Tracing fingerprint key in
// config snapshots: the microgateway encryption key, as for LLM API keys.
// The service container sets it.
var agentTracingKeyCrypto atomic.Value // cryptoHolder

type cryptoHolder struct{ c CryptoServiceInterface }

func setAgentTracingKeyCrypto(c CryptoServiceInterface) {
	agentTracingKeyCrypto.Store(cryptoHolder{c: c})
}

// installAgentTracingKey applies the fingerprint key of an applied snapshot
// (decision D4). The key is decrypted into memory only, never written to the
// edge database or logged. A snapshot without one (tracing off on the hub)
// removes it, and so does a key that cannot be decrypted: the edge then
// computes no fingerprints rather than wrong ones.
func installAgentTracingKey(k *pb.AgentTracingKeys) {
	if k == nil || k.KeyId == 0 || k.KeyId > 255 || k.HmacKeyEncrypted == "" {
		agenttrace.InstallEdgeKey(0, nil)
		return
	}
	h, _ := agentTracingKeyCrypto.Load().(cryptoHolder)
	if h.c == nil {
		log.Error().Msg("Agent Call Tracing: no decryption key for the fingerprint key in the configuration snapshot; edge fingerprints are off")
		agenttrace.InstallEdgeKey(0, nil)
		return
	}
	encoded, err := h.c.Decrypt(k.HmacKeyEncrypted)
	if err != nil {
		log.Error().Err(err).Msg("Agent Call Tracing: cannot decrypt the fingerprint key in the configuration snapshot; edge fingerprints are off")
		agenttrace.InstallEdgeKey(0, nil)
		return
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) < 32 {
		log.Error().Msg("Agent Call Tracing: the fingerprint key in the configuration snapshot is malformed; edge fingerprints are off")
		agenttrace.InstallEdgeKey(0, nil)
		return
	}
	agenttrace.InstallEdgeKey(byte(k.KeyId), key)
}
