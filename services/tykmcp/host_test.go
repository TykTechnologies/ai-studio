package tykmcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostConnectionValidate(t *testing.T) {
	token := func() string { return "key" }
	ok := HostConnection{URL: "http://localhost:3000", Token: token}
	assert.NoError(t, ok.Validate())

	for name, h := range map[string]HostConnection{
		"no token":        {URL: "http://localhost:3000"},
		"no url":          {Token: token},
		"relative url":    {URL: "/api", Token: token},
		"other scheme":    {URL: "ftp://dashboard", Token: token},
		"credentials":     {URL: "https://user:pw@dashboard.example.com", Token: token},
		"bad gateway url": {URL: "http://localhost:3000", GatewayURL: "gw.example.com", Token: token},
		"unknown mode":    {URL: "http://localhost:3000", Mode: "admin", Token: token},
		"overlong name":   {URL: "http://localhost:3000", Name: string(make([]byte, 201)), Token: token},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, h.Validate())
		})
	}
}
