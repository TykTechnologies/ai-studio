import apiClient from './apiClient';
import {
  fetchVendors,
  getEmbedderName,
  getEmbedderLogo,
  getEmbedderHelpText,
  getEmbedderDefaultModel,
  getEmbedderDefaultUrl,
  getVectorStoreName,
  getVectorStoreLogo,
  getVectorStoreHelpText,
  getVendorData,
  getEmbedderCodes,
  getVectorStoreCodes,
} from './vendorUtils';

// Mock apiClient
jest.mock('./apiClient');

describe('vendorUtils', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore?.();
  });

  describe('getEmbedderName', () => {
    test('should return name for known embedder', () => {
      expect(getEmbedderName('openai')).toBe('OpenAI');
    });

    test('should return name for ollama', () => {
      expect(getEmbedderName('ollama')).toBe('Ollama');
    });

    test('should return code when embedder not found', () => {
      expect(getEmbedderName('unknown')).toBe('unknown');
    });
  });

  describe('getEmbedderLogo', () => {
    test('should return logo path for known embedder', () => {
      expect(getEmbedderLogo('openai')).toBe('/logos/chatgpt-logo.png');
    });

    test('should return logo path for ollama', () => {
      expect(getEmbedderLogo('ollama')).toBe('/logos/ollama.png');
    });

    test('should return null for unknown embedder', () => {
      expect(getEmbedderLogo('unknown')).toBeNull();
    });
  });

  describe('getEmbedderHelpText', () => {
    test('should return help text for vertex embedder', () => {
      const helpText = getEmbedderHelpText('vertex');
      expect(helpText).toContain('project name');
      expect(helpText).toContain('region');
    });

    test('should return empty string for embedder with no help text', () => {
      expect(getEmbedderHelpText('openai')).toBe('');
    });

    test('should return default message for unknown embedder', () => {
      expect(getEmbedderHelpText('unknown')).toBe('No help text available.');
    });
  });

  describe('getEmbedderDefaultModel', () => {
    test('should return default model for openai', () => {
      expect(getEmbedderDefaultModel('openai')).toBe('text-embedding-3-small');
    });

    test('should return empty string for embedder without default model', () => {
      expect(getEmbedderDefaultModel('ollama')).toBe('');
    });

    test('should return empty string for unknown embedder', () => {
      expect(getEmbedderDefaultModel('unknown')).toBe('');
    });
  });

  describe('getEmbedderDefaultUrl', () => {
    test('should return default URL for openai', () => {
      expect(getEmbedderDefaultUrl('openai')).toBe('https://api.openai.com/v1');
    });

    test('should return empty string for embedder without default URL', () => {
      expect(getEmbedderDefaultUrl('ollama')).toBe('');
    });

    test('should return empty string for unknown embedder', () => {
      expect(getEmbedderDefaultUrl('unknown')).toBe('');
    });
  });

  describe('getVectorStoreName', () => {
    test('should return name for known vector store', () => {
      expect(getVectorStoreName('chroma')).toBe('Chroma');
    });

    test('should return name for pgvector', () => {
      expect(getVectorStoreName('pgvector')).toBe('pgvector');
    });

    test('should return name for pinecone', () => {
      expect(getVectorStoreName('pinecone')).toBe('Pinecone');
    });

    test('should return code when vector store not found', () => {
      expect(getVectorStoreName('unknown')).toBe('unknown');
    });
  });

  describe('getVectorStoreLogo', () => {
    test('should return logo path for known vector store', () => {
      expect(getVectorStoreLogo('chroma')).toBe('/logos/chroma-logo.png');
    });

    test('should return logo path for pgvector', () => {
      expect(getVectorStoreLogo('pgvector')).toBe('/logos/pg-logo.png');
    });

    test('should return null for unknown vector store', () => {
      expect(getVectorStoreLogo('unknown')).toBeNull();
    });
  });

  describe('getVectorStoreHelpText', () => {
    test('should return empty string for vector store with no help text', () => {
      expect(getVectorStoreHelpText('chroma')).toBe('');
    });

    test('should return default message for unknown vector store', () => {
      expect(getVectorStoreHelpText('unknown')).toBe('No help text available.');
    });
  });

  describe('getVendorData', () => {
    test('should return embedder data for known embedder', () => {
      const data = getVendorData('openai', 'embedder');
      expect(data.name).toBe('OpenAI');
      expect(data.logo).toBe('/logos/chatgpt-logo.png');
    });

    test('should return vector store data for known vector store', () => {
      const data = getVendorData('chroma', 'vectorStore');
      expect(data.name).toBe('Chroma');
      expect(data.logo).toBe('/logos/chroma-logo.png');
    });

    test('should return fallback data for unknown vendor', () => {
      const data = getVendorData('unknown', 'embedder');
      expect(data.name).toBe('unknown');
      expect(data.logo).toBeNull();
      expect(data.helpText).toBe('');
    });
  });

  describe('getEmbedderCodes', () => {
    test('should return array of embedder codes', () => {
      const codes = getEmbedderCodes();
      expect(codes).toContain('openai');
      expect(codes).toContain('ollama');
      expect(codes).toContain('vertex');
      expect(codes).toContain('google_ai');
    });
  });

  describe('getVectorStoreCodes', () => {
    test('should return array of vector store codes', () => {
      const codes = getVectorStoreCodes();
      expect(codes).toContain('chroma');
      expect(codes).toContain('pgvector');
      expect(codes).toContain('pinecone');
      expect(codes).toContain('redis');
      expect(codes).toContain('qdrant');
      expect(codes).toContain('weaviate');
    });
  });

  describe('fetchVendors', () => {
    test('should fetch vendors from API successfully', async () => {
      apiClient.get.mockResolvedValueOnce({
        data: { data: ['openai', 'ollama', 'custom_embedder'] },
      }).mockResolvedValueOnce({
        data: { data: ['chroma', 'pgvector', 'custom_store'] },
      });

      const result = await fetchVendors();

      expect(apiClient.get).toHaveBeenCalledWith('/vendors/embedders');
      expect(apiClient.get).toHaveBeenCalledWith('/vendors/vector-stores');
      expect(result.embedders).toBeDefined();
      expect(result.vectorStores).toBeDefined();
    });

    test('should handle API error and return default vendors', async () => {
      apiClient.get.mockRejectedValueOnce(new Error('Network error'));

      const result = await fetchVendors();

      expect(result.embedders).toBeDefined();
      expect(result.vectorStores).toBeDefined();
      expect(console.error).toHaveBeenCalledWith('Error fetching vendors:', expect.any(Error));
    });

    test('should handle missing data in API response', async () => {
      apiClient.get.mockResolvedValueOnce({ data: {} }).mockResolvedValueOnce({ data: {} });

      const result = await fetchVendors();

      expect(result.embedders).toBeDefined();
      expect(result.vectorStores).toBeDefined();
    });

    test('should preserve existing embedder metadata when API returns same codes', async () => {
      apiClient.get.mockResolvedValueOnce({
        data: { data: ['openai'] },
      }).mockResolvedValueOnce({
        data: { data: ['chroma'] },
      });

      const result = await fetchVendors();

      // Should preserve existing metadata for openai
      const openaiEmbedder = result.embedders.find(e => e.code === 'openai');
      expect(openaiEmbedder.name).toBe('OpenAI');
      expect(openaiEmbedder.logo).toBe('/logos/chatgpt-logo.png');
    });

    test('should create placeholder for new vendor codes from API', async () => {
      apiClient.get.mockResolvedValueOnce({
        data: { data: ['new_embedder'] },
      }).mockResolvedValueOnce({
        data: { data: ['new_store'] },
      });

      const result = await fetchVendors();

      const newEmbedder = result.embedders.find(e => e.code === 'new_embedder');
      expect(newEmbedder).toBeDefined();
      expect(newEmbedder.name).toBe('new_embedder');
      expect(newEmbedder.logo).toBeNull();
    });
  });
});
