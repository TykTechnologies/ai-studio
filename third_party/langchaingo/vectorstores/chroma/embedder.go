//go:build cgo

package chroma

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	chromaembeddings "github.com/amikos-tech/chroma-go/pkg/embeddings"
)

var _ chromaembeddings.EmbeddingFunction = chromaGoEmbedder{} // compile-time check

// chromaGoEmbedder adapts an 'embeddings.Embedder' to a 'chroma_go.EmbeddingFunction'.
type chromaGoEmbedder struct {
	embeddings.Embedder
}

func (e chromaGoEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([]chromaembeddings.Embedding, error) {
	_embeddings, err := e.Embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, err
	}
	_chrmembeddings := make([]chromaembeddings.Embedding, len(_embeddings))
	for i, emb := range _embeddings {
		_chrmembeddings[i] = chromaembeddings.NewEmbeddingFromFloat32(emb)
	}
	return _chrmembeddings, nil
}

func (e chromaGoEmbedder) EmbedQuery(ctx context.Context, text string) (chromaembeddings.Embedding, error) {
	_embedding, err := e.Embedder.EmbedQuery(ctx, text)
	if err != nil {
		return nil, err
	}
	return chromaembeddings.NewEmbeddingFromFloat32(_embedding), nil
}
