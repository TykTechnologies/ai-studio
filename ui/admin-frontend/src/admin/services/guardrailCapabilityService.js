import apiClient from "../utils/apiClient";

// The guardrail capability of an app (features/GuardrailExceptions.md): what
// a full administrator attests the app can do, which bounds how far guardrail
// exceptions may soften a verdict for it. Reading needs apps:read; setting a
// full administrator.

const path = (appId) => `/apps/${appId}/guardrail-capability`;

// getGuardrailCapability returns {value, effective, suggestion, reasons}.
export const getGuardrailCapability = async (appId) => {
  const response = await apiClient.get(path(appId));
  return response.data?.data;
};

// setGuardrailCapability attests value (read_only, read_tools, acting; ""
// clears it) and returns the same shape as getGuardrailCapability.
export const setGuardrailCapability = async (appId, value) => {
  const response = await apiClient.put(path(appId), { value });
  return response.data?.data;
};

export const CAPABILITY_OPTIONS = [
  {
    value: "read_only",
    label: "Read-only",
    help: "Only reads model output: no tools, datasources or MCP servers act for it.",
  },
  {
    value: "read_tools",
    label: "Reads with tools",
    help: "Its tools only read (GET requests, read-only MCP tools, datasources).",
  },
  {
    value: "acting",
    label: "Acting",
    help: "Its tools can change things (post, write, delete, send).",
  },
];

export const capabilityLabel = (value) =>
  CAPABILITY_OPTIONS.find((o) => o.value === value)?.label || "Not attested";
