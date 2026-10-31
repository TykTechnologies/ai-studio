package grpc

import "testing"

func TestEdgeCurrency(t *testing.T) {
	for in, want := range map[string]string{
		"":          "USD", // edges before the field was read
		"EUR":       "EUR",
		"USD":       "USD",
		"gbp":       "gbp", // as the model price has it, like embedded rows
		"EURO":      "USD",
		"E1R":       "USD",
		"€":         "USD",
		"US":        "USD",
		"USD; DROP": "USD",
	} {
		if got := edgeCurrency(in); got != want {
			t.Errorf("edgeCurrency(%q) = %q, want %q", in, got, want)
		}
	}
}
