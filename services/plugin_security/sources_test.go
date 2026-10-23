package plugin_security

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceOf(t *testing.T) {
	cases := map[string]PluginSource{
		"oci://registry.example.com/plugins/demo:1.0.0": SourceOCI,
		"grpc://plugin.example.com:50051":               SourceGRPC,
		"file:///opt/plugins/demo":                      SourceFile,
		"/opt/plugins/demo":                             SourceFile,
		"plugins/demo":                                  SourceFile,
		"./demo --flag":                                 SourceFile,
		"http://example.com/plugin":                     SourceFile, // run as a local executable, like any other command
		"https://example.com/plugin":                    SourceFile,
		"":                                              SourceFile,
		// Classified exactly as dispatched: no trimming, case folding or parsing.
		"OCI://registry.example.com/x":   SourceFile,
		" oci://registry.example.com/x":  SourceFile,
		"\toci://registry.example.com/x": SourceFile,
		"Grpc://plugin.example.com:1":    SourceFile,
		"oci:/registry.example.com/x":    SourceFile,
	}
	for command, want := range cases {
		assert.Equal(t, want, SourceOf(command), "SourceOf(%q)", command)
	}
}

func TestParseSourcePolicy(t *testing.T) {
	cases := map[string][]PluginSource{
		"":              {SourceOCI, SourceFile, SourceGRPC},
		"  ":            {SourceOCI, SourceFile, SourceGRPC},
		"oci":           {SourceOCI},
		" OCI , file ":  {SourceOCI, SourceFile},
		"grpc,oci,grpc": {SourceOCI, SourceGRPC},
		"file,oci,grpc": {SourceOCI, SourceFile, SourceGRPC},
		"oci,,":         {SourceOCI},
	}
	for csv, want := range cases {
		policy, err := ParseSourcePolicy(csv)
		require.NoError(t, err, csv)
		assert.Equal(t, want, policy.Allowed(), csv)
		assert.Equal(t, len(want) == 3, policy.AllowsAll(), csv)
	}

	_, err := ParseSourcePolicy("oci,ocii")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"ocii"`)
	assert.Contains(t, err.Error(), "oci, file, grpc")

	for _, empty := range []string{",", " , ,"} {
		_, err := ParseSourcePolicy(empty)
		assert.Error(t, err, "%q must not mean deny-all", empty)
	}
}

func TestSourcePolicyZeroValueAllowsAll(t *testing.T) {
	var policy SourcePolicy
	assert.True(t, policy.AllowsAll())
	assert.Equal(t, "all", policy.String())
	for _, command := range []string{"oci://r/x", "grpc://h:1", "/bin/x"} {
		assert.NoError(t, policy.Check(command))
	}
	assert.True(t, (&Config{}).SourcePolicy.AllowsAll(), "a Config without a policy allows all sources")
	assert.True(t, AllowAllSources().AllowsAll())
}

func TestSourcePolicyCheck(t *testing.T) {
	policy, err := ParseSourcePolicy("oci")
	require.NoError(t, err)
	policy = policy.WithSetting("AI_STUDIO_PLUGIN_ALLOWED_SOURCES")

	assert.NoError(t, policy.Check("oci://registry.example.com/plugins/demo:1"))

	err = policy.Check("/opt/plugins/demo")
	var notAllowed *ErrPluginSourceNotAllowed
	require.True(t, errors.As(err, &notAllowed))
	assert.Equal(t, SourceFile, notAllowed.Source)
	assert.Equal(t, []PluginSource{SourceOCI}, notAllowed.Allowed)
	assert.Equal(t, `🔒 SECURITY: plugin source "file" is not allowed by this installation (allowed: oci). Set AI_STUDIO_PLUGIN_ALLOWED_SOURCES to change this.`, err.Error())

	err = policy.Check("grpc://plugin.example.com:50051")
	require.True(t, errors.As(err, &notAllowed))
	assert.Equal(t, SourceGRPC, notAllowed.Source)
	assert.Equal(t, "oci", policy.String())
}
