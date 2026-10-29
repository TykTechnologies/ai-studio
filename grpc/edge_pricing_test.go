package grpc

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEdgePriceMultiplier(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	assert.Nil(t, edgePriceMultiplier(nil), "older edges send none")
	for _, bad := range []float64{-0.5, math.NaN(), math.Inf(1), 10.5} {
		assert.Nil(t, edgePriceMultiplier(f(bad)), "%v", bad)
	}
	for _, ok := range []float64{0, 0.5, 1, 10} {
		got := edgePriceMultiplier(f(ok))
		require.NotNil(t, got)
		assert.Equal(t, ok, *got)
	}
}

func TestEdgeRequestedModel(t *testing.T) {
	assert.Equal(t, "gemini-2.5-flash", edgeRequestedModel("gemini-2.5-flash"))
	assert.Empty(t, edgeRequestedModel(strings.Repeat("x", maxEdgeRequestedModel+1)))
}
