package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
)

// bufferedResponseCapture is used when response hooks are configured and need to modify responses
// This version buffers everything until Flush() is called
type bufferedResponseCapture struct {
	http.ResponseWriter
	statusCode int
	buffer     *bytes.Buffer
	header     http.Header
	written    bool
}

func newBufferedResponseCapture(w http.ResponseWriter) *bufferedResponseCapture {
	return &bufferedResponseCapture{
		ResponseWriter: w,
		buffer:         &bytes.Buffer{},
		header:         make(http.Header),
		written:        false,
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
	rc.buffer.Write(b)
	if rc.Header().Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(bytes.NewReader(rc.buffer.Bytes()))
		if err != nil {
			return 0, err
		}
		defer reader.Close()
		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return 0, err
		}
		rc.buffer = bytes.NewBuffer(decompressed)
	}
	// Don't write to client immediately - buffer for hooks to process
	return len(b), nil
}

func (rc *bufferedResponseCapture) CapturedBody() []byte {
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

// WriteToClient writes the buffered response to the client (call this after hooks modify the data)
func (rc *bufferedResponseCapture) WriteToClient() {
	if rc.written {
		return // Already written
	}
	
	// Apply headers to the actual response writer
	for k, v := range rc.header {
		rc.ResponseWriter.Header()[k] = v
	}
	
	// Write status code
	if rc.statusCode != 0 {
		rc.ResponseWriter.WriteHeader(rc.statusCode)
	}
	
	// Write body
	if rc.buffer.Len() > 0 {
		rc.ResponseWriter.Write(rc.buffer.Bytes())
	}
	
	rc.written = true
}