//go:build !cgo

package services

// chromaSupported mirrors data_session.ChromaSupported: Chroma needs cgo.
// A variable so tests can take either build's path.
var chromaSupported = false
