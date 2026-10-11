import { withBase } from "../../runtimeConfig";

const vendorData = {
  openai: {
    name: "OpenAI",
    logo: withBase("/logos/chatgpt-logo.png"),
    requiresAccessDetails: true,
    defaultEndpoint: "https://api.openai.com/v1",
    capabilities: ["chat", "embeddings", "decisions"],
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
  // Decision models over the System One wire format: TypeSafe Jev, or a
  // wire-compatible self-host (llama.cpp serving Kev, OpenJev, ...).
  systemone: {
    name: "System One (decision models)",
    logo: withBase("/logos/systemone.svg"),
    requiresAccessDetails: false,
    defaultEndpoint: "https://api.typesafe.ai",
    capabilities: ["decisions"],
  },
};

// What a vendor serves: "chat", "embeddings", "decisions". Vendors without a
// list are chat vendors. Mirrors switches.VendorCapabilities on the server.
export const getVendorCapabilities = (vendorCode) =>
  vendorData[vendorCode]?.capabilities || ["chat"];
export const vendorServesDecisions = (vendorCode) =>
  getVendorCapabilities(vendorCode).includes("decisions");
// A decision-only vendor cannot back chat rooms, agents or routers.
export const isDecisionOnlyVendor = (vendorCode) =>
  !getVendorCapabilities(vendorCode).includes("chat");
export const getChatVendorCodes = () =>
  Object.keys(vendorData).filter((code) => !isDecisionOnlyVendor(code));
// failoverCompatible reports whether an LLM of targetVendor can be a failover
// target for a primary of primaryVendor: they must serve a request kind in
// common (chat or decisions). The gateway skips any other rung, so offering
// it would only look like protection. An unknown primary vendor allows all.
export const failoverCompatible = (primaryVendor, targetVendor) => {
  if (!primaryVendor) return true;
  const kinds = ["chat", "decisions"];
  const primary = getVendorCapabilities(primaryVendor).filter((c) => kinds.includes(c));
  const target = getVendorCapabilities(targetVendor);
  return primary.some((c) => target.includes(c));
};
// chatCapableLLMs drops decision-only LLMs from an /llms API list.
export const chatCapableLLMs = (llms) =>
  (llms || []).filter(
    (llm) => !isDecisionOnlyVendor(llm?.attributes?.vendor ?? llm?.vendor)
  );

export const getVendorName = (vendorCode) =>
  vendorData[vendorCode]?.name || vendorCode;
export const getVendorLogo = (vendorCode) =>
  vendorData[vendorCode]?.logo || null;
export const getVendorCodes = () => Object.keys(vendorData);
export const vendorRequiresAccessDetails = (vendorCode) =>
  vendorData[vendorCode]?.requiresAccessDetails !== false;
export const getVendorDefaultEndpoint = (vendorCode) =>
  vendorData[vendorCode]?.defaultEndpoint || "";
