package budget

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInCurrency(t *testing.T) {
	for c, want := range map[string]bool{"": true, "USD": true, "usd": true, "EUR": false, "GBP": false} {
		assert.Equal(t, want, InCurrency(c), c)
	}
}

func TestMixedCurrencyError(t *testing.T) {
	err := MixedCurrencyError("app monthly budget", map[string]float64{"GBP": 1, "EUR": 58.32, "JPY": 0})
	assert.True(t, errors.Is(err, ErrMixedCurrency))
	assert.Equal(t, "budget cannot be enforced across currencies: the app monthly budget is in USD but this period has spend in EUR, GBP, which is not converted; set the model prices in USD", err.Error())
}
