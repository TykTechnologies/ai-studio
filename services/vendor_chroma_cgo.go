//go:build cgo

package services

// chromaSupported mirrors data_session.ChromaSupported: Chroma needs cgo.
// It is repeated here, not imported, so that services does not depend on
// data_session and every vector store client it links.
// A variable so tests can take either build's path.
var chromaSupported = true
