// One wording for the difference between a Tool and an MCP server, shown
// wherever an admin could pick the wrong one: the Tools list, the MCP servers
// list, the OpenAPI import wizard and the REST-API-to-MCP registration step.
//
// Tool: AI Studio serves it. MCP server: a Tyk Gateway serves it and AI Studio
// only catalogues it and issues keys.

export const TOOLS_BLURB =
  "Tools give chats and agents access to external services. A tool is built from an OpenAPI specification, and you choose which operations the LLM may use. A tool can also be opened to Apps over REST or MCP on the AI Studio gateway.";

export const TOOL_VS_MCP_SERVER = {
  tool: "A tool is served by AI Studio: its filters and analytics apply to every call, chats and agents can use it, and Apps reach it with their App credential.",
  mcpServer:
    "An MCP server is served by a Tyk Gateway: Tyk policies apply, Apps get a Tyk access key, and its traffic never passes through AI Studio.",
};

// Shown on the Tools side, pointing at MCP servers.
export const TOOLS_SEE_MCP_SERVERS = `${TOOL_VS_MCP_SERVER.tool} For MCP servers that run behind a Tyk Gateway, see MCP servers.`;

// Shown on the MCP servers side, pointing at Tools.
export const MCP_SERVERS_SEE_TOOLS = `${TOOL_VS_MCP_SERVER.mcpServer} To have AI Studio itself serve an OpenAPI API to chats and Apps, add it as a tool instead.`;
