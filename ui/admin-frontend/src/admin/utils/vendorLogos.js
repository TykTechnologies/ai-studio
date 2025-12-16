const vendorData = {
  openai: {
    name: "OpenAI",
    logo: "/logos/chatgpt-logo.png",
    requiresAccessDetails: true,
    defaultEndpoint: "https://api.openai.com/v1",
  },
  google_ai: {
    name: "Google AI",
    logo: "/logos/google-ai.png",
    requiresAccessDetails: true,
  },
  anthropic: {
    name: "Anthropic",
    logo: "/logos/anthropic.svg",
    requiresAccessDetails: true,
    defaultEndpoint: "https://api.anthropic.com/v1/",
  },
  vertex: {
    name: "Vertex AI",
    logo: "/logos/vertex.png",
    requiresAccessDetails: true,
  },
  huggingface: {
    name: "HuggingFace",
    logo: "/logos/hf-logo.svg",
    requiresAccessDetails: true,
  },
  ollama: {
    name: "Ollama",
    logo: "/logos/ollama.png",
    requiresAccessDetails: false,
  },

  // Add more vendors as needed
};

export const getVendorName = (vendorCode) =>
  vendorData[vendorCode]?.name || vendorCode;
export const getVendorLogo = (vendorCode) =>
  vendorData[vendorCode]?.logo || null;
export const getVendorCodes = () => Object.keys(vendorData);
export const vendorRequiresAccessDetails = (vendorCode) =>
  vendorData[vendorCode]?.requiresAccessDetails !== false;
export const getVendorDefaultEndpoint = (vendorCode) =>
  vendorData[vendorCode]?.defaultEndpoint || "";
