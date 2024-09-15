package proxy

import (
	"bytes"
	"net/http"
)

type responseCapture struct {
    http.ResponseWriter
    statusCode int
    buffer     *bytes.Buffer
}

func newResponseCapture(w http.ResponseWriter) *responseCapture {
    return &responseCapture{ResponseWriter: w, buffer: &bytes.Buffer{}}
}

func (rc *responseCapture) WriteHeader(statusCode int) {
    rc.statusCode = statusCode
    rc.ResponseWriter.WriteHeader(statusCode)
}

func (rc *responseCapture) Write(b []byte) (int, error) {
    rc.buffer.Write(b)
    return rc.ResponseWriter.Write(b)
}
