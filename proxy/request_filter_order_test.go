//go:build enterprise

package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Filters run top to bottom in the order stored on the LLM and the first
// block wins, so the rejection names whichever blocking filter the admin
// placed first. Swapping the chain swaps the name.
func TestRunRequestFilters_ChainOrderFirstBlockWins(t *testing.T) {
	p := newFilterTestProxy()

	blockAs := func(name string) *models.Filter {
		return &models.Filter{
			Name:   name,
			Script: []byte(`output := {block: true, message: "stop"}`),
		}
	}
	first, second := blockAs("first-block"), blockAs("second-block")

	run := func(filters ...*models.Filter) error {
		llm := &models.LLM{Vendor: models.OPENAI, Filters: filters}
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
		r := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(body))
		_, err := p.runRequestFilters(llm, r, []byte(body), "openai", "gpt-4")
		return err
	}

	err := run(first, second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "first-block")
	require.NotContains(t, err.Error(), "second-block")

	err = run(second, first)
	require.Error(t, err)
	require.Contains(t, err.Error(), "second-block")
	require.NotContains(t, err.Error(), "first-block")
}
