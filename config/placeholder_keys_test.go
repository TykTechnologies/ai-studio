package config

import (
	"strings"
	"testing"
)

// A quickstart placeholder left in a key is a configuration error, so
// studio.New stops before migrating and seeding the database under it
// (lsj.13).
func TestQuickstartPlaceholderKeysAreConfigErrors(t *testing.T) {
	env := map[string]string{
		"TYK_AI_SECRET_KEY":           "__REPLACE_WITH_RANDOM_SECRET_KEY__",
		"MICROGATEWAY_ENCRYPTION_KEY": "__REPLACE_WITH_RANDOM_32B_KEY__",
		"GRPC_AUTH_TOKEN":             "__REPLACE_WITH_RANDOM_TOKEN__",
	}
	conf := LoadFrom(func(k string) string { return env[k] })
	err := conf.Err()
	if err == nil {
		t.Fatal("expected configuration errors for placeholder keys")
	}
	for name := range env {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}

	env = map[string]string{
		"TYK_AI_SECRET_KEY":           "0123456789abcdef0123456789abcdef",
		"MICROGATEWAY_ENCRYPTION_KEY": "0123456789abcdef0123456789abcdef",
		"GRPC_AUTH_TOKEN":             "token",
	}
	conf = LoadFrom(func(k string) string { return env[k] })
	if err := conf.Err(); err != nil && strings.Contains(err.Error(), PlaceholderMarker) {
		t.Errorf("real keys reported as placeholders: %v", err)
	}
}
