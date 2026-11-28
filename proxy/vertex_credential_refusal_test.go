package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// A Vertex credential the gateway cannot use answers the same 502 and
// message on the native REST route as on the stream route (and /ai, /v1):
// "LLM credential not usable: <reason>", the reason naming the fix and
// carrying no key material. The refused request is recorded under the
// model's name, not "{model}:generateContent", so no price row is created
// for the method-suffixed name (midsommar-clean-1af.13, .14).
func TestVertexNative_CredentialRefusalMessageAndModel(t *testing.T) {
	requireSerial(t)
	t.Setenv(vertexVendor.ADCSetting, "")
	const wifSecret = "wif-audience-secret-part"
	wifKey := `{"type":"external_account","audience":"//iam.googleapis.com/` + wifSecret + `","subject_token_type":"urn:ietf:params:oauth:token-type:jwt","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"file":"/var/run/token"}}`

	cases := []struct {
		name     string
		key      string
		override bool
		wantText string
	}{
		{name: "no key", key: "", wantText: vertexVendor.ADCSetting},
		{name: "WIF without the opt-in", key: wifKey, wantText: vertexVendor.ADCSetting},
		{name: "token not minted", key: `{"type":"service_account"}`, override: true, wantText: "could not obtain a Google access token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.override {
				t.Cleanup(vertexVendor.SetTokenSourceOverride(func(*models.LLM) (oauth2.TokenSource, error) {
					return failingTokenSource{}, nil
				}))
			}
			h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(tc.key))

			for _, path := range []string{"/llm/rest/primary" + vertexNativePath, "/llm/stream/primary" + vertexStreamPath + "?alt=sse"} {
				resp, body := h.post(path, vertexNativeBody)
				assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "%s: %s", path, body)
				assert.Contains(t, string(body), "LLM credential not usable", path)
				assert.NotContains(t, string(body), "failed to make upstream request", path)
				assert.Contains(t, string(body), tc.wantText, path)
				assert.NotContains(t, string(body), wifSecret, "no key material")
				assert.NotContains(t, string(body), "invalid_grant", "token endpoint detail is not returned")
			}
			assert.Empty(t, h.primaryVendor.calls(), "nothing reached the upstream")

			h.proxy.waitForAnalyzers()
			for _, rec := range h.chatRecords() {
				assert.False(t, strings.Contains(rec.Name, ":"), "recorded model %q carries the method", rec.Name)
			}
			var suffixed int64
			require.NoError(t, h.db.Model(&models.ModelPrice{}).Where("model_name LIKE ?", "%:%").Count(&suffixed).Error)
			assert.Zero(t, suffixed, "no price row for a method-suffixed model name")
		})
	}
}
