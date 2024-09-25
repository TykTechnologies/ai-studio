import apiClient from "./apiClient";

let embedders = [
  {
    code: "openai",
    name: "OpenAI",
    logo: "/logos/chatgpt-logo.png",
    helpText: "",
  },
  {
    code: "ollama",
    name: "Ollama",
    logo: "/logos/ollama.png",
    helpText: "",
  },
  {
    code: "vertex",
    name: "Vertex AI",
    logo: "/logos/vertex.png",
    helpText:
      "The Vertex Embed client requires a project name and a region, add this as the Database Connection string as {project:location} (project ID followed by a colon, followed by the region)",
  },
  {
    code: "google_ai",
    name: "Google AI",
    logo: "/logos/google-ai.png",
    helpText: "",
  },
];

let vectorStores = [
  {
    code: "chroma",
    name: "Chroma",
    logo: "/logos/chroma-logo.png",
    helpText: "",
  },
  {
    code: "pgvector",
    name: "pgvector",
    logo: "/logos/pg-logo.png",
    helpText: "",
  },
  {
    code: "pinecone",
    name: "Pinecone",
    logo: "/logos/pinecone.png",
    helpText: "",
  },
  {
    code: "redis",
    name: "Redis",
    logo: "/logos/redis.svg",
    helpText: "",
  },
  {
    code: "qdrant",
    name: "Qdrant",
    logo: "/logos/qdrant.svg",
    helpText: "",
  },
  {
    code: "weaviate",
    name: "Weaviate",
    logo: "/logos/weaviate.png",
    helpText: "",
  },
];

export const fetchVendors = async () => {
  try {
    const [embeddersResponse, vectorStoresResponse] = await Promise.all([
      apiClient.get("/vendors/embedders"),
      apiClient.get("/vendors/vector-stores"),
    ]);

    // Update embedders and vectorStores if the API returns different data
    if (embeddersResponse.data.data) {
      embedders = embeddersResponse.data.data.map((code) => {
        const existingEmbedder = embedders.find((e) => e.code === code);
        return (
          existingEmbedder || { code, name: code, logo: null, helpText: "" }
        );
      });
    }
    if (vectorStoresResponse.data.data) {
      vectorStores = vectorStoresResponse.data.data.map((code) => {
        const existingVectorStore = vectorStores.find((v) => v.code === code);
        return (
          existingVectorStore || { code, name: code, logo: null, helpText: "" }
        );
      });
    }
    return { embedders, vectorStores };
  } catch (error) {
    console.error("Error fetching vendors:", error);
    return { embedders, vectorStores };
  }
};

export const getEmbedderName = (code) => {
  const embedder = embedders.find((e) => e.code === code);
  return embedder ? embedder.name : code;
};

export const getEmbedderLogo = (code) => {
  const embedder = embedders.find((e) => e.code === code);
  return embedder ? embedder.logo : null;
};

export const getEmbedderHelpText = (code) => {
  const embedder = embedders.find((e) => e.code === code);
  return embedder ? embedder.helpText : "No help text available.";
};

export const getVectorStoreName = (code) => {
  const vectorStore = vectorStores.find((v) => v.code === code);
  return vectorStore ? vectorStore.name : code;
};

export const getVectorStoreLogo = (code) => {
  const vectorStore = vectorStores.find((v) => v.code === code);
  return vectorStore ? vectorStore.logo : null;
};

export const getVectorStoreHelpText = (code) => {
  const vectorStore = vectorStores.find((v) => v.code === code);
  return vectorStore ? vectorStore.helpText : "No help text available.";
};

export const getVendorData = (code, type) => {
  const list = type === "embedder" ? embedders : vectorStores;
  return (
    list.find((item) => item.code === code) || {
      name: code,
      logo: null,
      helpText: "",
    }
  );
};

export const getEmbedderCodes = () => embedders.map((e) => e.code);
export const getVectorStoreCodes = () => vectorStores.map((v) => v.code);
