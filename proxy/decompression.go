package proxy

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/TykTechnologies/midsommar/v2/logger"
)

// decompressBuffer tries to decompress a gzipped buffer.
// It returns the original buffer's bytes if the content is not gzip or if any error occurs.
func decompressBuffer(body *bytes.Buffer, contentEncoding string) []byte {
	if contentEncoding != "gzip" {
		return body.Bytes()
	}

	// Create a new reader from the buffer's bytes because
	// gzip.NewReader might read from the buffer, and we want to return the
	// original, untouched buffer in case of an error.
	reader, err := gzip.NewReader(bytes.NewReader(body.Bytes()))
	if err != nil {
		logger.Warnf("Failed to create gzip reader, returning original buffer: %v", err)
		return body.Bytes()
	}
	defer reader.Close()

	decompressedData, err := io.ReadAll(reader)
	if err != nil {
		logger.Warnf("Error decompressing gzip stream, returning original buffer: %v", err)
		return body.Bytes()
	}

	return decompressedData
}
