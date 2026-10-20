package grpc

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// exportingFingerprinter stands in for the Enterprise key provider.
type exportingFingerprinter struct {
	agenttrace.StaticFingerprinter
}

func (e exportingFingerprinter) EdgeKey() (byte, []byte) { return e.ID, e.Key }

// Agent Call Tracing (D4): while tracing runs, the snapshot carries the
// derived HMAC key, encrypted with the microgateway key like LLM API keys.
// The plaintext key appears nowhere in the serialised snapshot, and the
// checksum is stable across regenerations but changes on rotation.
func TestSnapshot_AgentTracingKey(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")
	t.Cleanup(func() { agenttrace.SetFingerprinter(nil) })

	agenttrace.SetFingerprinter(nil)
	off, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Nil(t, off.AgentTracingKeys, "no key while tracing is off")

	key := bytes.Repeat([]byte{0xA7, 0x5C}, 16)
	agenttrace.SetFingerprinter(exportingFingerprinter{agenttrace.StaticFingerprinter{ID: 42, Key: key}})
	on, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.NotNil(t, on.AgentTracingKeys)
	assert.Equal(t, uint32(42), on.AgentTracingKeys.KeyId)
	got := decryptLikeMicrogateway(t, os.Getenv("MICROGATEWAY_ENCRYPTION_KEY"), on.AgentTracingKeys.HmacKeyEncrypted)
	assert.Equal(t, base64.StdEncoding.EncodeToString(key), got)
	assert.NotEqual(t, off.Checksum, on.Checksum, "turning tracing on marks edges pending")

	wire, err := proto.Marshal(on)
	require.NoError(t, err)
	js, err := protojson.Marshal(on)
	require.NoError(t, err)
	for _, form := range [][]byte{wire, js, []byte(on.String()), []byte(fmt.Sprintf("%v %+v", on, on))} {
		assert.False(t, bytes.Contains(form, key), "raw key in snapshot")
		assert.NotContains(t, string(form), base64.StdEncoding.EncodeToString(key))
		assert.NotContains(t, string(form), hex.EncodeToString(key))
	}

	again, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Equal(t, on.Checksum, again.Checksum, "regenerating keeps edges in sync")

	agenttrace.SetFingerprinter(exportingFingerprinter{agenttrace.StaticFingerprinter{ID: 43, Key: bytes.Repeat([]byte{1}, 32)}})
	rotated, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.NotEqual(t, on.Checksum, rotated.Checksum, "a rotation marks edges pending")

	// A Fingerprinter that exports no key (a test fake, or a future
	// provider) sends nothing.
	agenttrace.SetFingerprinter(agenttrace.StaticFingerprinter{ID: 9, Key: key})
	plain, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Nil(t, plain.AgentTracingKeys)
}
