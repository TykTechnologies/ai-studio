package secrets

import (
	"errors"
	"testing"
)

// A credential that did not resolve is refused before it is sent; a literal
// key, or none, is not.
func TestCheckCredential(t *testing.T) {
	for _, v := range []string{"$SECRET/MOCK_KEY", "$ENV/FOO_KEY", "$ENV/", "$ENC/abc", RefusedCredential} {
		if err := CheckCredential(v); !errors.Is(err, ErrUnresolvedCredential) {
			t.Errorf("%q: got %v, want ErrUnresolvedCredential", v, err)
		}
	}
	for _, v := range []string{"", "sk-live", "$literal-with-dollar", "user:pass"} {
		if err := CheckCredential(v); err != nil {
			t.Errorf("%q refused: %v", v, err)
		}
	}
}
