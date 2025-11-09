package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
)

// VendorDriverInfo represents information about a vendor driver
type VendorDriverInfo struct {
	Name              string
	Vendor            string
	Version           string
	Description       string
	SupportedFeatures []string
}

// GetAvailableLLMDrivers returns actual available LLM drivers from switches package
func (s *Service) GetAvailableLLMDrivers() ([]VendorDriverInfo, error) {
	drivers := make([]VendorDriverInfo, 0, len(switches.AVAILABLE_LLM_DRIVERS))

	for _, vendor := range switches.AVAILABLE_LLM_DRIVERS {
		var description string
		var features []string

		// Map vendor to description and supported features
		switch vendor {
		case models.OPENAI:
			description = "OpenAI GPT models with chat completion and embedding capabilities"
			features = []string{"chat", "completion", "embedding", "function-calling"}
		case models.ANTHROPIC:
			description = "Anthropic Claude models with advanced reasoning capabilities"
			features = []string{"chat", "completion", "function-calling", "large-context"}
		case models.OLLAMA:
			description = "Local Ollama models for self-hosted AI inference"
			features = []string{"chat", "completion", "embedding", "local-deployment"}
		case models.VERTEX:
			description = "Google Cloud Vertex AI models for enterprise applications"
			features = []string{"chat", "completion", "embedding", "enterprise-grade"}
		case models.GOOGLEAI:
			description = "Google AI models including Gemini and PaLM"
			features = []string{"chat", "completion", "multimodal", "reasoning"}
		default:
			description = "AI language model provider"
			features = []string{"chat", "completion"}
		}

		drivers = append(drivers, VendorDriverInfo{
			Name:              string(vendor),
			Vendor:            string(vendor),
			Version:           "1.0.0",
			Description:       description,
			SupportedFeatures: features,
		})
	}

	return drivers, nil
}

// GetAvailableEmbedders returns actual available embedders from switches package
func (s *Service) GetAvailableEmbedders() ([]VendorDriverInfo, error) {
	embedders := make([]VendorDriverInfo, 0, len(switches.AVAILABLE_EMBEDDERS))

	for _, vendor := range switches.AVAILABLE_EMBEDDERS {
		var description string
		var features []string

		// Map vendor to description and supported features
		switch vendor {
		case models.OPENAI:
			description = "OpenAI embedding models for text similarity and search"
			features = []string{"text-embedding", "document-embedding", "similarity-search"}
		case models.OLLAMA:
			description = "Local Ollama embedding models for self-hosted vector generation"
			features = []string{"text-embedding", "local-deployment", "privacy-focused"}
		case models.VERTEX:
			description = "Google Cloud Vertex AI embedding models for enterprise search"
			features = []string{"text-embedding", "multilingual", "enterprise-grade"}
		case models.GOOGLEAI:
			description = "Google AI embedding models with advanced language understanding"
			features = []string{"text-embedding", "multilingual", "semantic-search"}
		default:
			description = "Text embedding provider"
			features = []string{"text-embedding"}
		}

		embedders = append(embedders, VendorDriverInfo{
			Name:              string(vendor) + "-embeddings",
			Vendor:            string(vendor),
			Version:           "1.0.0",
			Description:       description,
			SupportedFeatures: features,
		})
	}

	return embedders, nil
}

// GetAvailableVectorStores returns available vector store implementations
func (s *Service) GetAvailableVectorStores() ([]VendorDriverInfo, error) {
	// Vector stores are typically configured via datasources in this system
	// Return the commonly supported vector store types
	vectorStores := []VendorDriverInfo{
		{
			Name:        "chroma",
			Vendor:      "chroma",
			Version:     "1.0.0",
			Description: "Open source vector database with local and remote deployment options",
			SupportedFeatures: []string{"vector-search", "metadata-filtering", "persistence", "collection-management"},
		},
		{
			Name:        "pinecone",
			Vendor:      "pinecone",
			Version:     "1.0.0",
			Description: "Managed vector database service with real-time indexing",
			SupportedFeatures: []string{"vector-search", "real-time-indexing", "managed-service", "high-performance"},
		},
		{
			Name:        "weaviate",
			Vendor:      "weaviate",
			Version:     "1.0.0",
			Description: "Open source vector search engine with GraphQL API",
			SupportedFeatures: []string{"vector-search", "hybrid-search", "graphql-api", "schema-management"},
		},
		{
			Name:        "qdrant",
			Vendor:      "qdrant",
			Version:     "1.0.0",
			Description: "High-performance vector database with rich filtering capabilities",
			SupportedFeatures: []string{"vector-search", "rich-filtering", "clustering", "quantization"},
		},
		{
			Name:        "milvus",
			Vendor:      "milvus",
			Version:     "1.0.0",
			Description: "Scalable vector database for AI applications",
			SupportedFeatures: []string{"vector-search", "scalability", "cloud-native", "multi-tenancy"},
		},
	}

	return vectorStores, nil
}