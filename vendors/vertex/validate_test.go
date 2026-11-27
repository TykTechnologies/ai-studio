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
	} {
		assert.NoError(t, ValidateEndpoint(ok), ok)
	}
	for _, bad := range []string{
		"", "proj", "a:b:c", ":us-central1", "proj:", "Proj:us-central1", "proj:US-CENTRAL1",
		"proj:us-central1/extra", "ftp://host", "https://", "proj:us_central1",
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
