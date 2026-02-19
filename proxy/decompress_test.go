package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestDecompressBody(t *testing.T) {
	original := []byte(`{"model":"gpt-4","choices":[{"message":{"content":"Hello"}}]}`)

	t.Run("gzip", func(t *testing.T) {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		w.Write(original)
		w.Close()

		result, err := decompressBody("gzip", buf.Bytes())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(result, original) {
			t.Fatalf("got %q, want %q", result, original)
		}
	})

	t.Run("brotli", func(t *testing.T) {
		var buf bytes.Buffer
		w := brotli.NewWriter(&buf)
		w.Write(original)
		w.Close()

		result, err := decompressBody("br", buf.Bytes())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(result, original) {
			t.Fatalf("got %q, want %q", result, original)
		}
	})

	t.Run("deflate", func(t *testing.T) {
		var buf bytes.Buffer
		w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		w.Write(original)
		w.Close()

		result, err := decompressBody("deflate", buf.Bytes())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(result, original) {
			t.Fatalf("got %q, want %q", result, original)
		}
	})

	t.Run("identity passthrough", func(t *testing.T) {
		result, err := decompressBody("identity", original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(result, original) {
			t.Fatalf("got %q, want %q", result, original)
		}
	})

	t.Run("empty encoding passthrough", func(t *testing.T) {
		result, err := decompressBody("", original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(result, original) {
			t.Fatalf("got %q, want %q", result, original)
		}
	})

	t.Run("empty data", func(t *testing.T) {
		result, err := decompressBody("gzip", []byte{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty result, got %d bytes", len(result))
		}
	})

	t.Run("unknown encoding passthrough", func(t *testing.T) {
		result, err := decompressBody("zstd", original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(result, original) {
			t.Fatalf("got %q, want %q", result, original)
		}
	})
}
