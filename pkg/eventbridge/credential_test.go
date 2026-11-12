package eventbridge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The payload's wire names are fixed: the control plane and the edge are
// built separately and must read each other's events.
func TestCredentialChangedWireFormat(t *testing.T) {
	b, err := json.Marshal(CredentialChanged{CredentialID: 11, Hash: "v1:ab", AppID: 42, Namespace: "ns", Active: true, Deleted: false})
	require.NoError(t, err)
	assert.JSONEq(t, `{"credential_id":11,"hash":"v1:ab","app_id":42,"namespace":"ns","active":true,"deleted":false}`, string(b))
	assert.Equal(t, "credential.changed", CredentialChangedTopic)

	c, err := ParseCredentialChanged(b)
	require.NoError(t, err)
	assert.Equal(t, CredentialChanged{CredentialID: 11, Hash: "v1:ab", AppID: 42, Namespace: "ns", Active: true}, c)

	_, err = ParseCredentialChanged(json.RawMessage(`{"credential_id":11}`))
	assert.ErrorIs(t, err, ErrCredentialChangedNoHash)
	_, err = ParseCredentialChanged(json.RawMessage(`nope`))
	assert.Error(t, err)
}
