package tykmcp

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The live fixtures are request bodies captured from Gateway 5.15.1 through
// Tyk Pump 1.16.0 (tests/tykmcp/oss, profile analytics: stdout pump + the
// Vector bridge in vector.toml) and through the http pump (profile
// httppump). One run each: a keyless MCP proxy and a keyed one (alias
// studio:app:42), each with initialize, tools/list, two get-weather calls and
// a call to an unknown tool, then the keyed one without a key (401) and with
// a bad key (403).
//
// Pump 1.16.0's stdout pump leaves out zero-valued mcp_stats fields
// (jsonrpc_error_code, the protocol versions) and the http pump sends them
// as 0 and "", so both must decode the same.
func TestTykAnalyticsRecord_DecodesLiveCaptures(t *testing.T) {
	hashedKey := regexp.MustCompile(`^[0-9a-f]{16}$`)
	for _, file := range []string{"pump_records_live_stdout_vector.json", "pump_records_live_http_pump.json"} {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + file)
			require.NoError(t, err)
			var recs []TykAnalyticsRecord
			require.NoError(t, json.Unmarshal(raw, &recs))
			require.Len(t, recs, 12)

			methods := map[string]int{}
			names := map[string]int{}
			for _, r := range recs {
				assert.True(t, r.MCPStats.IsMCP)
				assert.Zero(t, r.MCPStats.JSONRPCErrorCode, "upstream JSON-RPC errors are not in the record")
				assert.Equal(t, "", r.ProtocolVersion())
				assert.False(t, r.TimeStamp.IsZero())
				methods[r.MCPStats.JSONRPCMethod]++
				if r.MCPStats.PrimitiveType == "tool" {
					names[r.MCPStats.PrimitiveName]++
				}

				switch {
				case r.ResponseCode == 401 || r.ResponseCode == 403:
					// denied by the Gateway: recorded, but no key to attribute
					assert.Equal(t, "cap-keyed", r.APIID)
					assert.Equal(t, "00000000", r.APIKey)
					assert.Empty(t, r.Alias)
					assert.Equal(t, "get-weather", r.MCPStats.PrimitiveName)
				case r.APIID == "cap-keyed":
					assert.Equal(t, 200, r.ResponseCode)
					assert.Regexp(t, hashedKey, r.APIKey)
					assert.Equal(t, "studio:app:42", r.Alias)
					assert.Contains(t, r.Tags, "key-"+r.APIKey)
				default:
					assert.Equal(t, "cap-open", r.APIID)
					assert.Equal(t, "00000000", r.APIKey, "keyless calls carry the placeholder key")
				}
			}
			assert.Equal(t, map[string]int{"initialize": 2, "tools/list": 2, "tools/call": 8}, methods)
			assert.Equal(t, map[string]int{"get-weather": 6, "nope": 2}, names)

			// initialize and tools/list name themselves with no primitive type
			for _, r := range recs {
				if r.MCPStats.JSONRPCMethod != "tools/call" {
					assert.Equal(t, "", r.MCPStats.PrimitiveType)
					assert.Equal(t, r.MCPStats.JSONRPCMethod, r.MCPStats.PrimitiveName)
				}
			}
		})
	}
}
