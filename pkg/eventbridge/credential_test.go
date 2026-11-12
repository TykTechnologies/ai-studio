package eventbridge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
)

// The payload's wire names are fixed: the control plane and the edge are
// built separately and must read each other's events.
func TestCredentialChangedWireFormat(t *testing.T) {
	hash := "v1:" + strings.Repeat("ab", 32)
	require.Len(t, hash, credhash.Len)
	b, err := json.Marshal(CredentialChanged{CredentialID: 11, Hash: hash, AppID: 42, Namespace: "ns", Active: true, Deleted: false})
	require.NoError(t, err)
	assert.JSONEq(t, `{"credential_id":11,"hash":"`+hash+`","app_id":42,"namespace":"ns","active":true,"deleted":false}`, string(b))
	assert.Equal(t, "credential.changed", CredentialChangedTopic)

	c, err := ParseCredentialChanged(b)
	require.NoError(t, err)
	assert.Equal(t, CredentialChanged{CredentialID: 11, Hash: hash, AppID: 42, Namespace: "ns", Active: true}, c)

	_, err = ParseCredentialChanged(json.RawMessage(`{"credential_id":11}`))
	assert.ErrorIs(t, err, ErrCredentialChangedNoHash)
	_, err = ParseCredentialChanged(json.RawMessage(`nope`))
	assert.Error(t, err)
	// A hash of any other length cannot name a key: refused.
	for _, bad := range []string{"v1:ab", hash + "0", hash[:credhash.Len-1]} {
		_, err = ParseCredentialChanged(json.RawMessage(`{"credential_id":11,"hash":"` + bad + `"}`))
		assert.ErrorIs(t, err, ErrCredentialChangedBadHash, bad)
	}
}
