package vertexVendor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEndpoint(t *testing.T) {
	for _, ok := range []string{
		"my-project:us-central1", "my-project:global", "my-project:us", "my-project:eu",
		"example.com:my-project:europe-west4", " my-proj:asia-northeast1 ",
		"https://us-central1-aiplatform.googleapis.com", "https://psc.internal.example",
		// The fake-upstream form tests and local stacks use.
		"http://127.0.0.1:8089/v1/projects/p-1/locations/us-central1",
		"https://gw.example/vertex/projects/example.com:my-proj/locations/eu",
	} {
		assert.NoError(t, ValidateEndpoint(ok), ok)
	}
	for _, bad := range []string{
		"", "proj", "a:b:c", "a:b:us-central1", ":us-central1", "proj:", "Proj:us-central1", "proj:US-CENTRAL1",
		"proj:us-central1/extra", "ftp://x", "https://", "proj:us_central1",
		"https://vertex.example/v1/projects/p/locations",
	} {
		err := ValidateEndpoint(bad)
		require.Error(t, err, bad)
		assert.True(t, errors.Is(err, ErrInvalidEndpoint), bad)
		assert.Contains(t, err.Error(), "project:location", bad)
	}
}

func TestValidateKey(t *testing.T) {
	const secret = "s3cr3t-material"

	require.NoError(t, ValidateKey(mustJSON(t, serviceAccountJSON(t, "https://oauth2.googleapis.com/token"))))
	require.NoError(t, ValidateKey(`{"type":"external_account","audience":"//iam.googleapis.com/x","subject_token_type":"urn:ietf:params:oauth:token-type:jwt","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"file":"/var/run/token"}}`))
	require.NoError(t, ValidateKey(`{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`))
	require.NoError(t, ValidateKey("ya29.abc"))

	err := ValidateKey("")
	assert.True(t, IsWarning(err), "empty key is a warning")
	assert.Contains(t, err.Error(), ADCSetting)

	for _, bad := range []string{
		"AIza" + secret,
		`{"type":"service_account","private_key":"` + secret + `"}`,
		`{"type":"gdch_service_account","private_key":"` + secret + `"}`,
		`{not json ` + secret,
	} {
		err := ValidateKey(bad)
		require.Error(t, err)
		assert.False(t, IsWarning(err))
		assert.True(t, errors.Is(err, ErrInvalidCredential))
		assert.NotContains(t, err.Error(), secret)
	}
}

func TestValidateConfig(t *testing.T) {
	warn, err := ValidateConfig("proj", "ya29.t", true, true)
	assert.Nil(t, warn)
	assert.True(t, errors.Is(err, ErrInvalidEndpoint))

	warn, err = ValidateConfig("proj", "ya29.t", false, true)
	assert.NoError(t, err, "endpoint not being set is not checked")
	assert.Nil(t, warn)

	warn, err = ValidateConfig("my-proj:us-central1", "", true, true)
	assert.NoError(t, err)
	assert.True(t, IsWarning(warn), "key-less is a warning")

	warn, err = ValidateConfig("my-proj:us-central1", "$SECRET/VERTEX", true, true)
	assert.NoError(t, err, "a secret reference is resolved later")
	assert.Nil(t, warn)

	_, err = ValidateConfig("my-proj:us-central1", "junk", true, true)
	assert.True(t, errors.Is(err, ErrInvalidCredential))
	_, err = ValidateConfig("my-proj:us-central1", "junk", true, false)
	assert.NoError(t, err, "a kept key is not re-checked")
}
