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
