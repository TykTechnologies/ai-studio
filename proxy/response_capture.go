package proxy

import (
	"bytes"
	"net/http"
)

type responseCapture struct {
	http.ResponseWriter
	statusCode int
	buffer     *bytes.Buffer
	header     http.Header
}

func newResponseCapture(w http.ResponseWriter) *responseCapture {
	return &responseCapture{
		ResponseWriter: w,
		buffer:         &bytes.Buffer{},
		header:         make(http.Header),
	}
}

func (rc *responseCapture) Header() http.Header {
	return rc.header
}

func (rc *responseCapture) WriteHeader(statusCode int) {
	rc.statusCode = statusCode
	for k, v := range rc.header {
		rc.ResponseWriter.Header()[k] = v
	}
	rc.ResponseWriter.WriteHeader(statusCode)
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.buffer.Write(b)
	encoding := rc.Header().Get("Content-Encoding")
	if encoding != "" && encoding != "identity" {
		decompressed, err := decompressBody(encoding, rc.buffer.Bytes())
		if err != nil {
			return 0, err
		}
		rc.buffer = bytes.NewBuffer(decompressed)
	}
	return rc.ResponseWriter.Write(b)
}

func (rc *responseCapture) CapturedBody() []byte {
	return rc.buffer.Bytes()
}

