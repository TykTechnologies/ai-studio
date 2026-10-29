package budget

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Currency is the currency budgets are kept in. App, LLM and team budgets
// carry no currency of their own and costs are not converted, so a budget
// counts only spend priced in USD. Spend in another currency in a budget's
// period is not added to it: the budget refuses requests instead (fails
// closed) with ErrMixedCurrency, and the budget dashboards report it apart.
const Currency = "USD"

// InCurrencySQL selects llm_chat_records rows priced in the budget currency.
// Rows without a currency (recorded before it was) count as USD.
const InCurrencySQL = "UPPER(COALESCE(currency, '')) IN ('', 'USD')"

// CurrencyClassSQL is a row's currency for grouping: empty for the budget
// currency, else the upper-cased currency.
const CurrencyClassSQL = "CASE WHEN " + InCurrencySQL + " THEN '' ELSE UPPER(currency) END"

// InCurrency reports whether a chat record's currency is the budget currency.
func InCurrency(currency string) bool {
	return currency == "" || strings.EqualFold(currency, Currency)
}

// ErrMixedCurrency is wrapped by the error a budget check returns when the
// budget's period has spend in a currency other than the budget currency.
var ErrMixedCurrency = errors.New("budget cannot be enforced across currencies")

// MixedCurrencyError is the refusal for scope (for example "app monthly
// budget") whose period has spend in currencies, the other-currency amounts.
func MixedCurrencyError(scope string, currencies map[string]float64) error {
	return fmt.Errorf("%w: the %s is in %s but this period has spend in %s, which is not converted; set the model prices in %s",
		ErrMixedCurrency, scope, Currency, strings.Join(SortedCurrencies(currencies), ", "), Currency)
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
