package grpc

import (
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/logger"
)

// invalidEdgeCurrencies counts events recorded as USD for an invalid
// currency; logged on the first and every 100th.
var invalidEdgeCurrencies atomic.Uint64

// edgeCurrency is an edge analytics event's currency as the hub records it:
// the edge's value when it is a three-letter code (the currency of the model
// price the edge priced the request with), else USD. Edges before the field
// was read send it empty and priced in USD. Any other value is untrusted and
// is recorded as USD (and logged) rather than dropping the row, which would
// drop its spend from budgets.
func edgeCurrency(s string) string {
	if s == "" {
		return "USD"
	}
	if len(s) == 3 && isASCIILetter(s[0]) && isASCIILetter(s[1]) && isASCIILetter(s[2]) {
		return s
	}
	if n := invalidEdgeCurrencies.Add(1); n == 1 || n%100 == 0 {
		logger.Warnf("Edge analytics event has an invalid currency %q; recording it as USD (%d so far)", s, n)
	}
	return "USD"
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
