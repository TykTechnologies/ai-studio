import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import AppMCPConnect from "./AppMCPConnect";
import AppToolAccess from "./AppToolAccess";
import { MemoryRouter } from "react-router-dom";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));

const pubClient = require("../../admin/utils/pubClient").default;

const tool = (id, name, attrs) => ({ id: String(id), attributes: { name, slug: name.toLowerCase(), description: `${name} tool`, ...attrs } });

const weatherTool = tool(1, "Weather", {
  rest_access_enabled: true,
  mcp_access_enabled: true,
  rest_endpoint_url: "https://studio.example.com/tools/weather",
  mcp_endpoint_url: "https://studio.example.com/tools/weather/mcp",
});
const restOnlyTool = tool(2, "Billing", {
  rest_access_enabled: true,
  mcp_access_enabled: false,
  rest_endpoint_url: "https://studio.example.com/tools/billing",
});
const switchedOffTool = tool(3, "Legacy", { rest_access_enabled: false, mcp_access_enabled: false });

const crmServer = {
  id: 7,
  connection_id: 1,
  connection_name: "Prod",
  name: "CRM",
  slug: "crm",
  auth_mode: "auth_token",
  endpoint_url: "https://tyk.example.com/crm/",
  header_name: "X-API-Key",
  brokerable: true,
};

const credential = { secret: "app-secret", active: true };

const snippet = () => JSON.parse(screen.getByTestId("mcp-connect-snippet").textContent);

describe("AppMCPConnect", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({
      data: { servers: [crmServer], credentials: [], connections: [{ connection_id: 1, connection_name: "Prod", can_mint: true }] },
    });
  });

  it("shows tools and MCP servers together, each labelled with who serves it", async () => {
    render(<AppMCPConnect appId="5" credential={credential} showSecret tools={[weatherTool, restOnlyTool]} mcpServers={[crmServer]} />);

    const tools = screen.getByTestId("mcp-connect-tools");
    expect(tools).toHaveTextContent("Served by AI Studio");
    expect(tools).toHaveTextContent("https://studio.example.com/tools/weather/mcp");
    // A tool without MCP access has no place in this section.
    expect(tools).not.toHaveTextContent("Billing");

    expect(screen.getByTestId("mcp-connect-servers")).toHaveTextContent("Served by Tyk Gateway");
    await screen.findByTestId("mcp-connection-1");
  });

  it("builds one client configuration for both, with the right credential for each", async () => {
    render(<AppMCPConnect appId="5" credential={credential} showSecret tools={[weatherTool]} mcpServers={[crmServer]} />);
    await screen.findByTestId("mcp-connection-1");

    await waitFor(() =>
      expect(snippet().mcpServers.crm.args).toEqual(["mcp-remote", "https://tyk.example.com/crm/", "--header", "X-API-Key: <your Tyk access key>"]),
    );
    expect(snippet().mcpServers.weather.args).toEqual([
      "mcp-remote",
      "https://studio.example.com/tools/weather/mcp",
      "--header",
      "Authorization: Bearer app-secret",
    ]);
  });

  it("keeps the app secret out of the configuration until it is revealed", () => {
    render(<AppMCPConnect appId="5" credential={credential} showSecret={false} tools={[weatherTool]} mcpServers={[]} />);
    expect(screen.getByTestId("mcp-connect-snippet")).not.toHaveTextContent("app-secret");
    expect(snippet().mcpServers.weather.args[3]).toBe("Authorization: Bearer <your app credential>");
  });

  it("renders nothing for an app with no MCP endpoint", () => {
    const { container } = render(<AppMCPConnect appId="5" credential={credential} showSecret tools={[restOnlyTool]} mcpServers={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("AppToolAccess", () => {
  const renderTools = (tools) =>
    render(
      <MemoryRouter>
        <AppToolAccess tools={tools} />
      </MemoryRouter>,
    );

  it("shows the REST endpoint only for a tool with REST access on", () => {
    renderTools([weatherTool, tool(4, "Search", { rest_access_enabled: false, mcp_access_enabled: true })]);
    expect(screen.getByTestId("app-tool-rest-1")).toHaveTextContent("https://studio.example.com/tools/weather");
    expect(screen.queryByTestId("app-tool-rest-4")).not.toBeInTheDocument();
    expect(screen.getByTestId("app-tool-4")).toHaveTextContent("reached over MCP only");
  });

  it("explains a tool an administrator has since switched to chat only", () => {
    renderTools([switchedOffTool]);
    expect(screen.getByTestId("app-tool-unreachable-3")).toHaveTextContent("available in chat only");
    expect(screen.queryByRole("link", { name: "View Documentation" })).not.toBeInTheDocument();
  });
});
