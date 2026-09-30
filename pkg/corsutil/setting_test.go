package corsutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Studio's configured origin list wins over the environment; without it,
// the environment decides (the microgateway).
func TestSetAllowedOrigins(t *testing.T) {
	t.Setenv(EnvAllowedOrigins, "https://env.example.com")
	t.Cleanup(func() { allowedOrigins.Store(nil) })

	origin, explicit := AllowOrigin("https://env.example.com")
	assert.Equal(t, "https://env.example.com", origin)
	assert.True(t, explicit)

	SetAllowedOrigins("https://studio.example.com")
	origin, _ = AllowOrigin("https://env.example.com")
	assert.Empty(t, origin, "the environment's list no longer applies")
	origin, _ = AllowOrigin("https://studio.example.com")
	assert.Equal(t, "https://studio.example.com", origin)

	SetAllowedOrigins("")
	origin, explicit = AllowOrigin("https://anything.example.com")
	assert.Equal(t, "*", origin, "an empty configured list allows any origin")
	assert.False(t, explicit)
}
