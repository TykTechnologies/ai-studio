//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"math"
)

// MockEmbedder provides deterministic embeddings for testing.
// It generates vectors based on the hash of input text, ensuring
// the same text always produces the same vector.
type MockEmbedder struct {
	Dimensions int
}

// NewMockEmbedder creates a new MockEmbedder with the specified dimensions.
// Default dimension is 384 if not specified.
func NewMockEmbedder(dimensions int) *MockEmbedder {
	if dimensions <= 0 {
		dimensions = 384
	}
	return &MockEmbedder{Dimensions: dimensions}
}

// EmbedDocuments generates deterministic embeddings for multiple documents.
func (m *MockEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embeddings[i] = m.generateVector(text)
	}
	return embeddings, nil
}

// EmbedQuery generates a deterministic embedding for a query.
func (m *MockEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return m.generateVector(text), nil
}

// generateVector creates a deterministic vector from text using SHA256 hash.
func (m *MockEmbedder) generateVector(text string) []float32 {
	hash := sha256.Sum256([]byte(text))
	vector := make([]float32, m.Dimensions)

	// Use hash bytes to seed the vector values
	for i := 0; i < m.Dimensions; i++ {
		// Use different portions of the hash to generate each dimension
		hashIndex := i % 32
		seedByte := hash[hashIndex]

		// Mix in the dimension index for uniqueness
		seed := uint32(seedByte) ^ uint32(i*17)

		// Convert to float32 in range [-1, 1]
		vector[i] = (float32(seed)/255.0)*2.0 - 1.0
	}

	// Normalize the vector
	return normalizeVector(vector)
}

// normalizeVector normalizes a vector to unit length.
func normalizeVector(v []float32) []float32 {
	var sum float64
	for _, val := range v {
		sum += float64(val * val)
	}
	magnitude := float32(math.Sqrt(sum))

	if magnitude == 0 {
		return v
	}

	normalized := make([]float32, len(v))
	for i, val := range v {
		normalized[i] = val / magnitude
	}
	return normalized
}


// GenerateTestVectors creates deterministic test vectors.
func GenerateTestVectors(count, dimensions int) [][]float32 {
	embedder := NewMockEmbedder(dimensions)
	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		content := generateTestContent(i)
		vectors[i] = embedder.generateVector(content)
	}
	return vectors
}

// GenerateTestContents creates test content strings.
func GenerateTestContents(count int) []string {
	contents := make([]string, count)
	for i := 0; i < count; i++ {
		contents[i] = generateTestContent(i)
	}
	return contents
}

// GenerateTestMetadatas creates test metadata maps.
func GenerateTestMetadatas(count int) []map[string]any {
	metadatas := make([]map[string]any, count)
	for i := 0; i < count; i++ {
		metadatas[i] = map[string]any{
			"source":   "test",
			"doc_id":   i,
			"category": getTestCategory(i),
		}
	}
	return metadatas
}

// generateTestContent generates content for a test document.
func generateTestContent(index int) string {
	topics := []string{
		"The quick brown fox jumps over the lazy dog.",
		"Machine learning models can process natural language.",
		"Vector databases enable semantic search capabilities.",
		"Embeddings represent text as high-dimensional vectors.",
		"RAG systems combine retrieval with generation.",
	}
	return topics[index%len(topics)]
}

// getTestCategory returns a category based on index.
func getTestCategory(index int) string {
	categories := []string{"general", "ml", "database", "nlp", "rag"}
	return categories[index%len(categories)]
}

