package proxy

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func gzipCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("failed to write gzip data: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return buf.Bytes()
}

func TestDecompressBuffer(t *testing.T) {
	tests := []struct {
		name            string
		body            []byte
		contentEncoding string
		want            []byte
	}{
		{
			name:            "successful gzip decompression",
			body:            gzipCompress(t, []byte("hello world")),
			contentEncoding: "gzip",
			want:            []byte("hello world"),
		},
		{
			name:            "successful decompression of empty payload",
			body:            gzipCompress(t, []byte("")),
			contentEncoding: "gzip",
			want:            []byte(""),
		},
		{
			name:            "successful decompression of large payload",
			body:            gzipCompress(t, bytes.Repeat([]byte("abcdefghij"), 10000)),
			contentEncoding: "gzip",
			want:            bytes.Repeat([]byte("abcdefghij"), 10000),
		},
		{
			name:            "non-gzip content encoding returns original bytes",
			body:            []byte("plain text body"),
			contentEncoding: "deflate",
			want:            []byte("plain text body"),
		},
		{
			name:            "empty content encoding returns original bytes",
			body:            []byte("plain text body"),
			contentEncoding: "",
			want:            []byte("plain text body"),
		},
		{
			name:            "corrupted gzip data returns original bytes",
			body:            []byte{0x1f, 0x8b, 0x08, 0x00, 0xff, 0xff, 0xff},
			contentEncoding: "gzip",
			want:            []byte{0x1f, 0x8b, 0x08, 0x00, 0xff, 0xff, 0xff},
		},
		{
			name:            "truncated gzip stream returns original bytes",
			body:            gzipCompress(t, []byte("hello world"))[:10],
			contentEncoding: "gzip",
			want:            gzipCompress(t, []byte("hello world"))[:10],
		},
		{
			name:            "empty buffer with gzip encoding returns original bytes",
			body:            []byte{},
			contentEncoding: "gzip",
			want:            []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(tt.body)
			got := decompressBuffer(buf, tt.contentEncoding)

			if !bytes.Equal(got, tt.want) {
				gotStr := string(got)
				wantStr := string(tt.want)
				if len(gotStr) > 100 {
					gotStr = gotStr[:100] + "..."
				}
				if len(wantStr) > 100 {
					wantStr = wantStr[:100] + "..."
				}
				t.Errorf("decompressBuffer() = %q, want %q", gotStr, wantStr)
			}
		})
	}
}
