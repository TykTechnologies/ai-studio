package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"

	"github.com/andybalholm/brotli"
)

// decompressBody decompresses a response body based on the Content-Encoding header value.
// Returns the original data unchanged if the encoding is empty, "identity", or unrecognized.
func decompressBody(encoding string, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case "br":
		reader := brotli.NewReader(bytes.NewReader(data))
		return io.ReadAll(reader)
	case "deflate":
		reader := flate.NewReader(bytes.NewReader(data))
		defer reader.Close()
		return io.ReadAll(reader)
	default:
		return data, nil
	}
}
