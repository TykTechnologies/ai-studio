// One builder for the MCP client configuration the portal hands out.
//
// Two kinds of endpoint can be reached over MCP, and a developer pastes both
// into the same client file:
//   - a Tool, served by the AI Studio gateway and opened with the App
//     credential ("Authorization: Bearer <secret>");
//   - an MCP server, served by a Tyk Gateway and opened with a Tyk access key
//     in whichever header the proxy expects.
// They used to be built by two unrelated snippets that disagreed on the header
// convention. Everything goes through here now.

export const SERVED_BY = {
  studio: "Served by AI Studio",
  tyk: "Served by Tyk Gateway",
};

// Placeholders for a value the page cannot show: a Tyk key is revealed once
// when it is minted, and an App secret may be hidden.
export const APP_CREDENTIAL_PLACEHOLDER = "<your app credential>";
export const TYK_KEY_PLACEHOLDER = "<your Tyk access key>";

// configName makes a stable, client-safe key for the "mcpServers" map.
export const configName = (value, fallback = "server") => {
  const name = String(value || "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return name || fallback;
};

/** An entry for a Tool: App credential as a bearer token. */
export const toolEntry = (tool, secret) => ({
  name: configName(tool.slug || tool.name, "tool"),
  url: tool.mcp_endpoint_url,
  headerName: "Authorization",
  headerValue: `Bearer ${secret || APP_CREDENTIAL_PLACEHOLDER}`,
});

/** An entry for a Tyk-managed MCP server: the Tyk key in the proxy's header. */
export const mcpServerEntry = (server, key) => ({
  name: configName(server.slug || server.name, "mcp-server"),
  url: server.endpoint_url,
  headerName: server.header_name || "Authorization",
  headerValue: key || TYK_KEY_PLACEHOLDER,
});

// Entries without a URL are dropped; a name used twice gets a suffix so one
// does not silently replace the other in the map.
export const mcpClientConfigObject = (entries = []) => {
  const mcpServers = {};
  entries
    .filter((entry) => entry && entry.url)
    .forEach((entry) => {
      let name = entry.name;
      for (let n = 2; mcpServers[name]; n += 1) name = `${entry.name}-${n}`;
      mcpServers[name] = {
        command: "npx",
        args: ["mcp-remote", entry.url, "--header", `${entry.headerName}: ${entry.headerValue}`],
      };
    });
  return { mcpServers };
};

export const mcpClientConfig = (entries) => JSON.stringify(mcpClientConfigObject(entries), null, 2);
