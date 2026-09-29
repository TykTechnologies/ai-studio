import { withBase } from "../../runtimeConfig";

const vendorData = {
  openai: {
    name: "OpenAI",
    logo: withBase("/logos/chatgpt-logo.png"),
    requiresAccessDetails: true,
    defaultEndpoint: "https://api.openai.com/v1",
  },
  google_ai: {
    name: "Google AI",
    logo: withBase("/logos/google-ai.png"),
    requiresAccessDetails: true,
  },
  anthropic: {
    name: "Anthropic",
    logo: withBase("/logos/anthropic.svg"),
    requiresAccessDetails: true,
    defaultEndpoint: "https://api.anthropic.com/v1/",
  },
  vertex: {
    name: "Vertex AI",
    logo: withBase("/logos/vertex.png"),
    requiresAccessDetails: true,
  },
  huggingface: {
    name: "HuggingFace",
    logo: withBase("/logos/hf-logo.svg"),
    requiresAccessDetails: true,
  },
  ollama: {
    name: "Ollama",
    logo: withBase("/logos/ollama.png"),
    requiresAccessDetails: false,
  },
  bedrock: {
    name: "AWS Bedrock",
    logo: withBase("/logos/aws-bedrock.svg"),
    requiresAccessDetails: true,
    defaultEndpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
  },
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
