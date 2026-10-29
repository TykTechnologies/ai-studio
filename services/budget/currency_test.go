package budget

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMixedCurrency(t *testing.T) {
	assert.False(t, MixedCurrency(nil))
	assert.False(t, MixedCurrency(map[string]float64{"EUR": 5}))
	assert.False(t, MixedCurrency(map[string]float64{"EUR": 5, "USD": 0}))
	assert.True(t, MixedCurrency(map[string]float64{"EUR": 5, "USD": 1}))
}

func TestMixedCurrencyError(t *testing.T) {
	err := MixedCurrencyError("app monthly budget", map[string]float64{"USD": 18.74, "EUR": 58.32, "JPY": 0})
	assert.True(t, errors.Is(err, ErrMixedCurrency))
	assert.Equal(t, "budget cannot be enforced across currencies: the app monthly budget's period has spend in EUR, USD, which is not converted; price its models in one currency", err.Error())
}

func TestSplitCurrency(t *testing.T) {
	c, total, mixed := SplitCurrency(nil)
	assert.Equal(t, "", c)
	assert.Zero(t, total)
	assert.Nil(t, mixed)

	c, total, mixed = SplitCurrency(map[string]float64{"EUR": 2.5, "USD": 0})
	assert.Equal(t, "EUR", c)
	assert.Equal(t, 2.5, total)
	assert.Nil(t, mixed)

	spend := map[string]float64{"EUR": 2, "USD": 1}
	c, total, mixed = SplitCurrency(spend)
	assert.Equal(t, "", c)
	assert.Zero(t, total)
	assert.Equal(t, spend, mixed)
}
