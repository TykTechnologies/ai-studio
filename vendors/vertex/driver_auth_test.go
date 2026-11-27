package vertexVendor

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/googleai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type oauthTS = oauth2.TokenSource

func typeName(v any) string { return fmt.Sprintf("%T", v) }

// captureWarnings collects slog output for the test and resets the
// once-per-owner deprecation memory.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	adcDeprecationWarned = sync.Map{}
	t.Cleanup(func() {
		slog.SetDefault(prev)
		adcDeprecationWarned = sync.Map{}
	})
	return &buf
}

// authOptionCount counts the token-source client options in opts.
func authOptionCount(opts []googleai.Option) int {
	o := googleai.DefaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	n := 0
	for _, co := range o.ClientOptions {
		if strings.Contains(strings.ToLower(typeName(co)), "tokensource") {
			n++
		}
	}
	return n
}

func TestStudioDriver_CredentialJSONUsesExplicitCredentials(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "")
	logs := captureWarnings(t)
	llm := &models.LLM{ID: 40, Name: "chat", Vendor: models.VERTEX, APIEndpoint: "proj:us-central1", APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))}

	opts, err := driverAuth(llm.APIKey, "LLM chat", "llm:40", func() (oauthTS, error) { return TokenSource(llm) })
	require.NoError(t, err)
	assert.Equal(t, 1, authOptionCount(opts), "the client gets the chain's token source")

	d, err := setupVertexDriver(llm, nil)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Zero(t, calls.Load(), "no host ADC lookup")
	assert.Empty(t, logs.String(), "no deprecation warning with a key")
}

func TestStudioDriver_EmptyKeyKeepsSDKADCWithDeprecationWarning(t *testing.T) {
	allowLocalEndpoints(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "")
	logs := captureWarnings(t)

	for i := 0; i < 3; i++ {
		opts, err := driverAuth("", "LLM \"chat\" (ID 41)", "llm:41", func() (oauthTS, error) {
			t.Fatal("the credential chain is not used for a key-less driver while the setting is off")
			return nil, nil
		})
		require.NoError(t, err)
		assert.Empty(t, opts, "no auth option: the SDK's own ADC lookup, as before")
	}
	assert.Equal(t, 1, strings.Count(logs.String(), "deprecated"), "warned once per owner")
	assert.Contains(t, logs.String(), ADCSetting)
	assert.Zero(t, calls.Load())

	_, err := driverAuth("", "embedder 9", "embedder:9", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(logs.String(), "deprecated"), "another owner warns again")
}

func TestStudioDriver_EmptyKeyWithADCOnUsesChainAndNoWarning(t *testing.T) {
	allowLocalEndpoints(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "true")
	logs := captureWarnings(t)
	llm := &models.LLM{ID: 42, Name: "chat", Vendor: models.VERTEX, APIEndpoint: "proj:us-central1"}

	opts, err := driverAuth("", "LLM chat", "llm:42", func() (oauthTS, error) { return TokenSource(llm) })
	require.NoError(t, err)
	assert.Equal(t, 1, authOptionCount(opts))
	assert.EqualValues(t, 1, calls.Load(), "host credentials through the chain's gate")
	assert.Empty(t, logs.String(), "no deprecation warning when the operator opted in")
}

func TestStudioDriver_InvalidKeyRefused(t *testing.T) {
	allowLocalEndpoints(t)
	captureWarnings(t)
	_, err := setupVertexDriver(&models.LLM{ID: 43, Name: "chat", Vendor: models.VERTEX, APIEndpoint: "proj:us-central1", APIKey: "AIza-not-a-vertex-credential"}, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "AIza-not-a-vertex-credential")
}

func TestStudioEmbedder_CredentialChain(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "")
	logs := captureWarnings(t)

	spec := &models.EmbedderSpec{EmbedderID: 5, Vendor: models.VERTEX, Endpoint: "proj:us-central1", Model: "text-embedding-004", APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))}
	c, err := setupVertexEmbedClient(spec)
	require.NoError(t, err)
	require.NotNil(t, c)
	ts, err := EmbedderTokenSource(spec)
	require.NoError(t, err)
	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "sa-1", tok.AccessToken)
	assert.Zero(t, calls.Load())
	assert.Empty(t, logs.String())

	// Key-less: SDK ADC as before, with the warning.
	_, err = driverAuth("", "embedder 6", "embedder:6", func() (oauthTS, error) { return EmbedderTokenSource(&models.EmbedderSpec{EmbedderID: 6}) })
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "deprecated")
	assert.Zero(t, calls.Load())

	// Invalid key: refused, not passed to the SDK as an API key.
	_, err = setupVertexEmbedClient(&models.EmbedderSpec{EmbedderID: 7, Vendor: models.VERTEX, Endpoint: "proj:us-central1", APIKey: "not-a-credential"})
	require.Error(t, err)
}
