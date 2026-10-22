package extract

import (
	"bytes"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// Bodies are untrusted: no input may panic, and canonical output must be a
// fixed point (canonicalising it again changes nothing).
func FuzzExtract(f *testing.F) {
	for _, fx := range toolfixtures.ByFormat(requestFormats...) {
		f.Add(fx.Format, fx.Body)
	}
	f.Add(FormatOpenAI, bigOpenAIResponse())
	f.Add(FormatAnthropic, []byte(`{"a":[1,2.50,{"b":null}],"c":"\u0000"}`))
	f.Fuzz(func(t *testing.T, format string, body []byte) {
		_, _ = ParseRequest(format, body, testFP)
		_, _ = ParseResponse(format, body, testFP)
		_, _ = ArgsFingerprint(testFP, body)
		if c, err := Canonicalize(body); err == nil {
			again, err := Canonicalize(c)
			if err != nil || !bytes.Equal(c, again) {
				t.Fatalf("canonical form is not a fixed point: %q -> %q (%v)", c, again, err)
			}
		}
	})
}
