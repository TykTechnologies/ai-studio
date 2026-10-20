package tykmcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The decoder reads Tyk Pump's AnalyticsRecord JSON: MCP stats, latency,
// network and tags come through, and fields it does not declare (raw
// bodies, IP, geo, fields from newer pumps) are ignored.
func TestTykAnalyticsRecord_DecodesPumpJSON(t *testing.T) {
	raw, err := os.ReadFile("testdata/pump_records.json")
	require.NoError(t, err)
	var recs []TykAnalyticsRecord
	require.NoError(t, json.Unmarshal(raw, &recs))
	require.Len(t, recs, 2)

	r := recs[0]
	assert.Equal(t, "POST", r.Method)
	assert.Equal(t, 200, r.ResponseCode)
	assert.Equal(t, "e3b0c44298fc1c14", r.APIKey)
	assert.Equal(t, "6a1f0c2b9d", r.APIID)
	assert.Equal(t, "5e9d9544a1dcd60001d0ed20", r.OrgID)
	assert.Equal(t, "studio:app:12", r.Alias)
	assert.True(t, r.TimeStamp.Equal(time.Date(2026, 10, 20, 9, 15, 2, 123456000, time.UTC)))
	assert.Equal(t, TykLatency{Total: 41, Upstream: 37, Gateway: 4}, r.Latency)
	assert.Equal(t, int64(187), r.Network.BytesIn)
	assert.Equal(t, int64(2048), r.Network.BytesOut)
	assert.True(t, r.MCPStats.IsMCP)
	assert.Equal(t, "tools/call", r.MCPStats.JSONRPCMethod)
	assert.Equal(t, "tool", r.MCPStats.PrimitiveType)
	assert.Equal(t, "get_forecast", r.MCPStats.PrimitiveName)
	assert.Equal(t, int64(-32602), r.MCPStats.JSONRPCErrorCode)
	assert.Equal(t, "2025-06-18", r.ProtocolVersion())
	assert.Equal(t, "sess-123", r.SessionID())
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", r.TraceID())

	assert.False(t, recs[1].MCPStats.IsMCP)
	assert.Equal(t, "", recs[1].SessionID())
	assert.Equal(t, "", recs[1].ProtocolVersion())

	// Nothing the struct holds can carry a body or the client IP.
	out, err := json.Marshal(r)
	require.NoError(t, err)
	for _, s := range []string{"203.0.113.9", "UE9TVC", "raw_request", "ip_address", "geo"} {
		assert.NotContains(t, string(out), s)
	}
}

func TestTykAnalyticsRecord_ProtocolVersionFallsBackToDeclared(t *testing.T) {
	r := TykAnalyticsRecord{MCPStats: TykMCPStats{DeclaredProtocolVersion: "2025-03-26"}}
	assert.Equal(t, "2025-03-26", r.ProtocolVersion())
}

func TestIngestToken(t *testing.T) {
	tok, hash, display, err := NewIngestToken()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(tok, IngestTokenPrefix))
	assert.Len(t, hash, 64)
	assert.NotContains(t, hash, tok)
	assert.Equal(t, tok[:len(display)], display)
	assert.Less(t, len(display), len(tok)-30, "the display prefix must not reveal the secret")
	assert.Equal(t, HashIngestToken(tok), hash)

	assert.True(t, IngestTokenMatches(tok, hash))
	assert.True(t, IngestTokenMatches(tok, strings.ToUpper(hash)))
	assert.False(t, IngestTokenMatches(tok+"x", hash))
	assert.False(t, IngestTokenMatches("", hash))
	assert.False(t, IngestTokenMatches(tok, ""))

	tok2, hash2, _, err := NewIngestToken()
	require.NoError(t, err)
	assert.NotEqual(t, tok, tok2)
	assert.NotEqual(t, hash, hash2)
}
