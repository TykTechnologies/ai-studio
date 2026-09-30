package licensing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigKey(t *testing.T) {
	assert.Equal(t, "env-licence", Config{LicenseKey: "env-licence"}.Key())
	n := 0
	c := Config{LicenseKey: "env-licence", LicenseSource: func() string { n++; return " host-licence \n" }}
	assert.Equal(t, "host-licence", c.Key(), "the source wins, trimmed")
	c.Key()
	assert.Equal(t, 2, n, "read on every call, so a renewal is seen")
	assert.Equal(t, "", Config{LicenseSource: func() string { return "" }}.Key())
}
