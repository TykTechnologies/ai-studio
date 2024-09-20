const vendorData = {
  openai: {
    name: "OpenAI",
    logo: "/logos/chatgp-logo.png",
  },
  google_ai: {
    name: "Google AI",
    logo: "/logos/google-ai.png",
  },
  anthropic: {
    name: "Anthropic",
    logo: "/logos/anthropic.svg",
  },
  vertex: {
    name: "Vertex AI",
    logo: "/logos/vertex.png",
  },
  huggingface: {
    name: "HuggingFace",
    logo: "/logos/hf-logo.svg",
  },
  ollama: {
    name: "Ollama",
    logo: "/logos/ollama.png",
  },

  // Add more vendors as needed
};

export const getVendorName = (vendorCode) =>
  vendorData[vendorCode]?.name || vendorCode;
export const getVendorLogo = (vendorCode) =>
  vendorData[vendorCode]?.logo || null;
export const getVendorCodes = () => Object.keys(vendorData);
