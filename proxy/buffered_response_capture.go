package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
)

// bufferedResponseCapture is used when response hooks are configured and need to modify responses
// This version buffers everything until Flush() is called
type bufferedResponseCapture struct {
	http.ResponseWriter
	statusCode      int
	buffer          *bytes.Buffer
	header          http.Header
	written         bool
	gzipDecompressed bool // Track if we've already decompressed gzip
}

func newBufferedResponseCapture(w http.ResponseWriter) *bufferedResponseCapture {
	return &bufferedResponseCapture{
		ResponseWriter:   w,
		buffer:           &bytes.Buffer{},
		header:           make(http.Header),
		written:          false,
		gzipDecompressed: false,
	}
}

func (rc *bufferedResponseCapture) Header() http.Header {
	return rc.header
}

func (rc *bufferedResponseCapture) WriteHeader(statusCode int) {
	rc.statusCode = statusCode
	// Don't write to client immediately - buffer for hooks to process
}

func (rc *bufferedResponseCapture) Write(b []byte) (int, error) {
	// Just buffer the data as-is, don't try to decompress yet
	// Decompression happens in WriteToClient() when the full response is available
	rc.buffer.Write(b)
	return len(b), nil
}

func (rc *bufferedResponseCapture) CapturedBody() []byte {
	// Decompress gzip content if present for analytics
	if !rc.gzipDecompressed && rc.header.Get("Content-Encoding") == "gzip" && rc.buffer.Len() > 0 {
		reader, err := gzip.NewReader(bytes.NewReader(rc.buffer.Bytes()))
		if err == nil {
			decompressed, err := io.ReadAll(reader)
			reader.Close()
			if err == nil {
				return decompressed
			}
		}
	}
	return rc.buffer.Bytes()
}

// ModifyHeaders allows hooks to modify response headers before sending to client
func (rc *bufferedResponseCapture) ModifyHeaders(headers map[string]string) {
	if rc.written {
		return // Too late to modify
	}
	
	// Clear existing headers and set new ones
	rc.header = make(http.Header)
	for key, value := range headers {
		rc.header.Set(key, value)
	}
}

// ModifyBody allows hooks to modify response body before sending to client
func (rc *bufferedResponseCapture) ModifyBody(body []byte) {
	if rc.written {
		return // Too late to modify
	}
	
	rc.buffer = bytes.NewBuffer(body)
}

// ModifyStatusCode allows hooks to modify response status code before sending to client
func (rc *bufferedResponseCapture) ModifyStatusCode(statusCode int) {
	if rc.written {
		return // Too late to modify
	}
	
	rc.statusCode = statusCode
}

// Flush implements http.Flusher interface
// This is called by httputil.ReverseProxy during response copy
// We don't actually flush during buffering, just satisfy the interface
func (rc *bufferedResponseCapture) Flush() {
	// During buffering phase, we don't flush to the underlying writer
	// This prevents the reverse proxy from panicking when it tries to flush
	// The actual flush happens in WriteToClient()
}

// WriteToClient writes the buffered response to the client (call this after hooks modify the data)
func (rc *bufferedResponseCapture) WriteToClient() {
	if rc.written {
		return // Already written
	}

	// Decompress gzip content if present and not already decompressed
	if !rc.gzipDecompressed && rc.header.Get("Content-Encoding") == "gzip" && rc.buffer.Len() > 0 {
		reader, err := gzip.NewReader(bytes.NewReader(rc.buffer.Bytes()))
		if err == nil {
			decompressed, err := io.ReadAll(reader)
			reader.Close()
			if err == nil {
				rc.buffer = bytes.NewBuffer(decompressed)
				// Remove Content-Encoding header since we've decompressed the data
				rc.header.Del("Content-Encoding")
				rc.gzipDecompressed = true
			}
		}
	}

	bufLen := rc.buffer.Len()

	// CRITICAL: Set Content-Length FIRST, before applying other headers
	// HTTP/2 requires explicit content length for proper framing
	if bufLen > 0 {
		rc.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", bufLen))
	}

	// Apply headers to the actual response writer (skip Content-Length as we just set it)
	for k, values := range rc.header {
		// Skip Content-Length as we've already set it correctly based on buffer size
		if k == "Content-Length" {
			continue
		}
		// Delete existing values and set new ones
		rc.ResponseWriter.Header().Del(k)
		for _, v := range values {
			rc.ResponseWriter.Header().Add(k, v)
		}
	}

	// Write status code
	if rc.statusCode != 0 {
		rc.ResponseWriter.WriteHeader(rc.statusCode)
	}

	// Write body
	if bufLen > 0 {
		rc.ResponseWriter.Write(rc.buffer.Bytes())
	}

	// CRITICAL: Flush to ensure response is sent through Gin/HTTP2/proxies
	// Without this, Gin's wrapped ResponseWriter may not commit the response
	// especially critical for HTTP/2 connections through tunnels like Cloudflare
	if f, ok := rc.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}

	rc.written = true
}