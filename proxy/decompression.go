package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/andybalholm/brotli"
)

func decompressResponseBody(data []byte, contentEncoding string) ([]byte, error) {
	if len(data) == 0 || contentEncoding == "" {
		return data, nil
	}

	switch strings.ToLower(contentEncoding) {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer func() {
			if err := reader.Close(); err != nil {
				logger.Errorf("failed to close gzip reader: %v", err)
			}
		}()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip data: %v", err)
		}

		return decompressed, nil

	case "br", "brotli":
		decompressed, err := io.ReadAll(brotli.NewReader(bytes.NewReader(data)))
		if err != nil {
			return nil, fmt.Errorf("failed to decompress brotli data: %v", err)
		}

		return decompressed, nil

	default:
		logger.Errorf("Decompression is not supported for %s, returning original data", contentEncoding)
		return data, nil
	}
}
