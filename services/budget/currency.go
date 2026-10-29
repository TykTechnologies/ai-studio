package budget

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Budgets carry no currency of their own and costs are not converted. A
// budget's currency is implied by its spend: when all of a budget's spend in
// its period is in one currency, the budget is in that currency and counts it
// as it is. When the period mixes currencies the spend cannot be added up, so
// the budget refuses requests (fails closed) with ErrMixedCurrency.

// DefaultCurrency is the currency of chat records that carry none (recorded
// before the currency was).
const DefaultCurrency = "USD"

// CurrencySQL is an llm_chat_records row's currency for grouping:
// upper-cased, with an empty currency read as DefaultCurrency.
const CurrencySQL = "CASE WHEN COALESCE(currency, '') = '' THEN 'USD' ELSE UPPER(currency) END"

// ErrMixedCurrency is wrapped by the error a budget check returns when the
// budget's period has spend in more than one currency.
var ErrMixedCurrency = errors.New("budget cannot be enforced across currencies")

// MixedCurrency reports whether spend (by currency) has more than one
// currency with spend above 0.
func MixedCurrency(spend map[string]float64) bool {
	return len(SortedCurrencies(spend)) > 1
}

// MixedCurrencyError is the refusal for scope (for example "app monthly
// budget") whose period has spend in several currencies.
func MixedCurrencyError(scope string, spend map[string]float64) error {
	return fmt.Errorf("%w: the %s's period has spend in %s, which is not converted; price its models in one currency",
		ErrMixedCurrency, scope, strings.Join(SortedCurrencies(spend), ", "))
}

// SplitCurrency reads spend by currency as a budget does: the one currency
// and its total, or, when it mixes currencies, mixed (the spend by currency,
// which cannot be added up) and a total of 0. No spend is ("", 0, nil).
func SplitCurrency(spend map[string]float64) (currency string, total float64, mixed map[string]float64) {
	switch currencies := SortedCurrencies(spend); len(currencies) {
	case 0:
		return "", 0, nil
	case 1:
		return currencies[0], spend[currencies[0]], nil
	default:
		return "", 0, spend
	}
}

// SortedCurrencies returns the currencies with spend above 0, sorted.
func SortedCurrencies(spend map[string]float64) []string {
	out := make([]string, 0, len(spend))
	for c, v := range spend {
		if v > 0 {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// AddCurrencies adds b's amounts to a, allocating a when needed; amounts of 0
// are left out.
func AddCurrencies(a, b map[string]float64) map[string]float64 {
	for c, v := range b {
		if v == 0 {
			continue
		}
		if a == nil {
			a = map[string]float64{}
		}
		a[c] += v
	}
	return a
}
