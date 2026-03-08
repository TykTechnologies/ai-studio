package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretPreserveReference(t *testing.T) {
	s := &Secret{VarName: "MY_KEY", Value: "decrypted-value"}

	assert.Equal(t, "decrypted-value", s.GetValue())

	s.PreserveReference()
	assert.Equal(t, "$SECRET/MY_KEY", s.GetValue())
}
