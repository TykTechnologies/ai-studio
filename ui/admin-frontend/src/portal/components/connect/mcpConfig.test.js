import {
  APP_CREDENTIAL_PLACEHOLDER,
  TYK_KEY_PLACEHOLDER,
  configName,
  mcpClientConfigObject,
  mcpServerEntry,
  toolEntry,
} from "./mcpConfig";

describe("mcpConfig", () => {
  it("opens a tool with the app credential as a bearer token", () => {
    const entry = toolEntry({ slug: "weather", mcp_endpoint_url: "https://gw/tools/weather/mcp" }, "s3cret");
    expect(entry).toEqual({
      name: "weather",
      url: "https://gw/tools/weather/mcp",
      headerName: "Authorization",
      headerValue: "Bearer s3cret",
    });
    expect(toolEntry({ slug: "weather", mcp_endpoint_url: "u" }).headerValue).toBe(`Bearer ${APP_CREDENTIAL_PLACEHOLDER}`);
  });

  it("opens a Tyk MCP server with its key in the header the proxy expects", () => {
    const entry = mcpServerEntry({ slug: "crm", endpoint_url: "https://tyk/crm/", header_name: "X-API-Key" }, "k1");
    expect(entry).toEqual({ name: "crm", url: "https://tyk/crm/", headerName: "X-API-Key", headerValue: "k1" });
    // A Tyk key is shown once, so a page built later can only offer a placeholder.
    expect(mcpServerEntry({ name: "CRM", endpoint_url: "u" }).headerValue).toBe(TYK_KEY_PLACEHOLDER);
    expect(mcpServerEntry({ name: "CRM", endpoint_url: "u" }).headerName).toBe("Authorization");
  });

  it("puts tools and MCP servers into one client file, in the same shape", () => {
    const config = mcpClientConfigObject([
      toolEntry({ slug: "weather", mcp_endpoint_url: "https://gw/tools/weather/mcp" }, "s3cret"),
      mcpServerEntry({ slug: "crm", endpoint_url: "https://tyk/crm/" }, "k1"),
    ]);
    expect(config).toEqual({
      mcpServers: {
        weather: { command: "npx", args: ["mcp-remote", "https://gw/tools/weather/mcp", "--header", "Authorization: Bearer s3cret"] },
        crm: { command: "npx", args: ["mcp-remote", "https://tyk/crm/", "--header", "Authorization: k1"] },
      },
    });
  });

  it("drops entries without a URL and keeps both of two entries with one name", () => {
    const config = mcpClientConfigObject([
      toolEntry({ slug: "search", mcp_endpoint_url: "https://gw/tools/search/mcp" }, "a"),
      mcpServerEntry({ slug: "search", endpoint_url: "https://tyk/search/" }, "b"),
      mcpServerEntry({ slug: "no-url" }, "c"),
      null,
    ]);
    expect(Object.keys(config.mcpServers)).toEqual(["search", "search-2"]);
  });

  it("makes a client-safe name", () => {
    expect(configName("Café & Bar")).toBe("caf-bar");
    expect(configName("", "tool")).toBe("tool");
  });
});
