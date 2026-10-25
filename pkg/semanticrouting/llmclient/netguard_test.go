package llmclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stubs in these tests are httptest servers on 127.0.0.1, and
// internal-range blocking is on by default: turn it off for the package.
// TestClients_AllowInternalHost turns it back on.
func init() { netguardtest.AllowInternalForPackage() }

// With blocking on, an LLM or inline embedder on an internal host is reached
// only when its own "Allow internal host" switch is on (w4e): embeddings and
// the judge's completion alike.
func TestClients_AllowInternalHost(t *testing.T) {
	netguardtest.BlockInternal(t)
	t.Setenv(netguard.EnvAllowInternal, "")
	t.Setenv(netguard.EnvAllowedInternalHosts, "")
	stub := &openAIStub{}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer srv.Close()

	llms := map[uint]*models.LLM{
		1: {ID: 1, Vendor: models.OPENAI, APIEndpoint: srv.URL + "/v1", APIKey: "sk"},
		2: {ID: 2, Vendor: models.OPENAI, APIEndpoint: srv.URL + "/v1", APIKey: "sk", AllowInternalHost: true},
	}
	c := New(func(id uint) (*models.LLM, error) {
		if l, ok := llms[id]; ok {
			return l, nil
		}
		return nil, errors.New("not found")
	})
	_, err := c.Embed(context.Background(), sr.ModelRef{LLMID: 1, Model: "m"}, []string{"a"})
	assert.Error(t, err, "an LLM without the switch reached an internal host")
	_, err = c.Complete(context.Background(), sr.ModelRef{LLMID: 1, Model: "m"}, "s", "u")
	assert.Error(t, err)
	_, err = c.Embed(context.Background(), sr.ModelRef{LLMID: 2, Model: "m"}, []string{"a"})
	require.NoError(t, err)
	_, err = c.Complete(context.Background(), sr.ModelRef{LLMID: 2, Model: "m"}, "s", "u")
	require.NoError(t, err)

	ref := sr.ModelRef{EmbedderID: 4, Vendor: "openai", Endpoint: srv.URL + "/v1", APIKey: "sk", Model: "m"}
	_, err = c.Embed(context.Background(), ref, []string{"a"})
	assert.Error(t, err, "an inline embedder without the switch reached an internal host")
	ref.AllowInternalHost = true
	_, err = c.Embed(context.Background(), ref, []string{"a"})
	require.NoError(t, err)
}
