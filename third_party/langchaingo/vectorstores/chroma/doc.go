//go:build cgo

// Tyk: every file of this package is cgo-only, because chroma-go's v2 client
// loads its default embedding function (a tokenizer and the ONNX runtime)
// through cgo. AI Studio only imports it from data_session/chroma.go, which
// is cgo-only too.

// Package chroma contains an implementation of the VectorStore interface that connects to an external Chroma database.
package chroma
