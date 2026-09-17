import { generateSlug } from "../../admin/components/wizards/quick-start/utils";

// Where a tool is served on the AI Studio gateway.
//
// The API sends both URLs (rest_endpoint_url, mcp_endpoint_url) and the slug
// they are built from. Prefer them: the slug is computed on the server with
// transliteration ("Café" is served under "cafe"), which a browser-side regex
// cannot reproduce. The fallbacks only cover a server that predates those
// fields or has no display URL configured.

const trimSlash = (value) => String(value || "").replace(/\/+$/, "");

const toolSlug = (attrs = {}) => attrs.slug || generateSlug(attrs.name || "");

export const toolRestEndpoint = (attrs = {}, baseUrl = "") => {
  if (attrs.rest_endpoint_url) return attrs.rest_endpoint_url;
  const base = trimSlash(baseUrl);
  return base ? `${base}/tools/${toolSlug(attrs)}` : "";
};

export const toolMcpEndpoint = (attrs = {}, baseUrl = "") => {
  if (attrs.mcp_endpoint_url) return attrs.mcp_endpoint_url;
  const rest = toolRestEndpoint({ ...attrs, rest_endpoint_url: "" }, baseUrl);
  return rest ? `${rest}/mcp` : "";
};

// Access flags. A server that predates them sends neither, and every tool it
// lists is reachable both ways, so "not false" is the safe reading.
export const toolRestEnabled = (attrs = {}) => attrs.rest_access_enabled !== false;
export const toolMcpEnabled = (attrs = {}) => attrs.mcp_access_enabled !== false;
