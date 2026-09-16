import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../admin/utils/testTheme";
import AssetDetail from "./AssetDetail";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../../config", () => ({ getConfig: () => ({ proxyURL: "http://gw.example.com" }) }));

const pubClient = require("../../admin/utils/pubClient").default;

const server = {
  type: "mcp_server",
  id: "12",
  attributes: {
    name: "Weather MCP proxy",
    short_description: "Weather tools",
    kind: "remote",
    privacy_score: 35,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-02T00:00:00Z",
    catalogs: [],
    tags: ["weather"],
    access_granted_via_app: true,
    auth_mode: "auth_token",
    auth_header: "Authorization",
    endpoint_url: "https://gw.example.com/weather/mcp",
    endpoint_urls: { "edge-eu": "https://eu.gw.example.com/weather/mcp" },
    gateway_tags: ["edge-eu"],
    brokerable: true,
    primitives: [
      { type: "tool", name: "get-weather", description: "Current conditions" },
      { type: "resource", name: "weather://stations/*" },
    ],
  },
};

const apps = {
  data: [
    { id: "1", attributes: { name: "Weather app", llm_ids: [], datasource_ids: [], tool_ids: [], mcp_server_ids: [12], is_active: true, credential_active: true } },
    { id: "2", attributes: { name: "Other app", llm_ids: [9], datasource_ids: [], tool_ids: [], mcp_server_ids: [], is_active: true, credential_active: false } },
  ],
};

const renderDetail = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/portal/catalog/mcp-servers/12"]}>
        <Routes>
          <Route path="/portal/catalog/mcp-servers/:id" element={<AssetDetail type="mcp_server" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("AssetDetail (MCP server)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/catalog/mcp-servers/12") return Promise.resolve({ data: { data: server } });
      if (url === "/common/apps") return Promise.resolve({ data: apps });
      return Promise.reject(new Error("unexpected " + url));
    });
  });

  it("shows how to connect, the primitives and the apps using it", async () => {
    renderDetail();
    expect(await screen.findByText("Weather MCP proxy")).toBeInTheDocument();
    expect(screen.getByText(/API key \(Tyk access key\)/)).toBeInTheDocument();
    expect(screen.getByText("https://gw.example.com/weather/mcp")).toBeInTheDocument();
    expect(screen.getByText("https://eu.gw.example.com/weather/mcp")).toBeInTheDocument();
    expect(screen.getByTestId("asset-gateway-tags")).toHaveTextContent("edge-eu");
    expect(screen.getByTestId("mcp-access-note")).toHaveTextContent(/request a Tyk access key/);
    const prims = screen.getByTestId("asset-primitives");
    expect(prims).toHaveTextContent("get-weather");
    expect(prims).toHaveTextContent("weather://stations/*");
    expect(screen.getByTestId("asset-build-app")).toHaveTextContent("Build app");
    const usingApps = screen.getByTestId("asset-apps");
    expect(usingApps).toHaveTextContent("Weather app");
    expect(usingApps).not.toHaveTextContent("Other app");
  });

  it("offers no Build app for a server AI Studio does not broker", async () => {
    const oauthServer = {
      ...server,
      attributes: { ...server.attributes, access_granted_via_app: false, brokerable: false, auth_mode: "oauth21", auth_header: "", oauth: { authorization_servers: ["https://auth.example.com"] } },
    };
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/catalog/mcp-servers/12") return Promise.resolve({ data: { data: oauthServer } });
      if (url === "/common/apps") return Promise.resolve({ data: apps });
      return Promise.reject(new Error("unexpected " + url));
    });
    renderDetail();
    expect(await screen.findByText("Weather MCP proxy")).toBeInTheDocument();
    expect(screen.queryByTestId("asset-build-app")).not.toBeInTheDocument();
    expect(screen.getByTestId("mcp-access-note")).toHaveTextContent(/does not broker access/);
    expect(screen.getByText(/https:\/\/auth.example.com/)).toBeInTheDocument();
    expect(screen.queryByTestId("asset-apps")).not.toBeInTheDocument();
    expect(screen.queryByText("Your apps")).not.toBeInTheDocument();
  });
});
