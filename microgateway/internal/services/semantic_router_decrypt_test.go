package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Standalone embedder keys reach the edge encrypted inside the router's
// configuration; the service decrypts them with the gateway's crypto once
// it is wired, and refuses until then.
func TestSemanticRouterService_DecryptsEmbedderKeys(t *testing.T) {
	s := NewSemanticRouterService(nil)
	_, err := s.decryptKey("enc")
	assert.Error(t, err)

	s.SetDecrypter(func(c string) (string, error) { return strings.TrimPrefix(c, "enc:"), nil })
	got, err := s.decryptKey("enc:sk-1")
	require.NoError(t, err)
	assert.Equal(t, "sk-1", got)
}
