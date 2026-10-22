package proxy

import (
	"bytes"
	"math/rand"
	"testing"
)

// sseEventEndReference is the original definition: the earlier of the first
// "\n\n" and the first "\r\n\r\n".
func sseEventEndReference(b []byte) int {
	best := -1
	if i := bytes.Index(b, []byte("\n\n")); i >= 0 {
		best = i + 2
	}
	if i := bytes.Index(b, []byte("\r\n\r\n")); i >= 0 && (best < 0 || i+4 < best) {
		best = i + 4
	}
	return best
}

func TestSSEEventEndMatchesReference(t *testing.T) {
	for _, s := range []string{"", "\n", "\n\n", "a\n\nb", "a\r\n\r\nb", "a\n\r\n", "a\r\n\nb", "\r\n\r\n\n\n",
		"data: x\r\ndata: y\r\n\r\n", "a\rb\n\n", "\r\n\r", "x\n\r\n\r\n"} {
		if got, want := sseEventEnd([]byte(s)), sseEventEndReference([]byte(s)); got != want {
			t.Errorf("%q: got %d, want %d", s, got, want)
		}
	}
	rng := rand.New(rand.NewSource(1))
	alphabet := []byte("a\r\n:")
	for n := 0; n < 20000; n++ {
		b := make([]byte, rng.Intn(16))
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		if got, want := sseEventEnd(b), sseEventEndReference(b); got != want {
			t.Fatalf("%q: got %d, want %d", b, got, want)
		}
	}
}

func FuzzSSEEventEnd(f *testing.F) {
	f.Add([]byte("data: a\r\n\r\ndata: b\n\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if got, want := sseEventEnd(b), sseEventEndReference(b); got != want {
			t.Fatalf("%q: got %d, want %d", b, got, want)
		}
	})
}
