package data_session

import (
	"slices"
	"testing"
)

// AVAILABLE_VECTOR_STORES lists Chroma exactly when this build can use it.
func TestAvailableVectorStoresFollowChromaSupport(t *testing.T) {
	if got := slices.Contains(AVAILABLE_VECTOR_STORES, VECTOR_CHROMA); got != ChromaSupported {
		t.Fatalf("chroma listed = %v, ChromaSupported = %v", got, ChromaSupported)
	}
}
