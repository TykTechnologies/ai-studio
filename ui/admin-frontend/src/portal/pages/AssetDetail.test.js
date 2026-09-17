import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
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

const llm = {
  type: "llm",
  id: "7",
  attributes: {
    name: "Acme OpenAI",
    short_description: "Fast general model",
    long_description: "Longer words about the model.",
    kind: "openai",
    privacy_score: 40,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-02T00:00:00Z",
    catalogs: [{ id: "1", name: "Default" }],
    tags: [],
    default_model: "gpt-4o",
    allowed_models: ["gpt-4o", "^gpt-4o-mini$"],
    models: [
      { name: "gpt-4o", is_default: true, input_price_per_million: 2.5, output_price_per_million: 10, currency: "USD" },
      { name: "gpt-4o-mini", is_default: false, input_price_per_million: 0.15, output_price_per_million: 0.6, currency: "USD" },
    ],
    metadata: { region: "eu" },
  },
  governed_metadata: [{ key: "owner", label: "Owner", type: "string", value: "Platform team" }],
};

const apps = {
  data: [
    { id: "1", attributes: { name: "Support bot", llm_ids: [7], datasource_ids: [], tool_ids: [], is_active: true, credential_active: true } },
    { id: "2", attributes: { name: "Other app", llm_ids: [9], datasource_ids: [], tool_ids: [], is_active: true, credential_active: false } },
  ],
};

const renderDetail = (path, type = "llm") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/portal/catalog/llms/:id" element={<AssetDetail type={type} />} />
          <Route path="/portal/catalog/tools/:id" element={<AssetDetail type="tool" />} />
          <Route path="/portal/catalog/resources/:pluginId/:slug/:instanceId" element={<AssetDetail type="plugin_resource" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

// The "More" modal used to show a heading and a vendor. The detail page shows
// what a developer needs to decide: models (and the allow list), privacy,
// the URL, the catalogs it comes through, and which of their apps use it.
describe("AssetDetail", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/catalog/llms/7") return Promise.resolve({ data: { data: llm } });
      if (url === "/common/apps") return Promise.resolve({ data: apps });
      if (url === "/common/catalog/llms/8") return Promise.reject({ response: { status: 404 } });
      if (url === "/common/catalog/tools/4") {
        return Promise.resolve({
          data: { data: { type: "tool", id: "4", attributes: { name: "Weather API", kind: "rest", privacy_score: 10, operations: ["getForecast", "getAlerts"], catalogs: [] } } },
        });
      }
      if (url === "/common/catalog/resources/7/agent/ast_9") {
        return Promise.resolve({
          data: {
            data: {
              type: "plugin_resource",
              id: "ast_9",
              attributes: {
                name: "Support Triage Agent",
                kind: "7:agent",
                kind_label: "Agent",
                privacy_score: 20,
                catalogs: [],
                tags: [],
                access_granted_via_app: false,
                portal_detail_url: "/portal/plugins/asset-catalog#/assets/ast_9",
                resource_type: { plugin_id: 7, slug: "agent", name: "Agent", access_granted_via_app: false },
              },
            },
          },
        });
      }
      if (url === "/common/catalog/resources/3/mcp_servers/srv-1") {
        return Promise.resolve({
          data: {
            data: {
              type: "plugin_resource",
              id: "srv-1",
              attributes: {
                name: "Support MCP",
                kind: "3:mcp_servers",
                kind_label: "MCP Servers",
                privacy_score: 20,
                catalogs: [],
                tags: [],
                access_granted_via_app: true,
                resource_type: { plugin_id: 3, slug: "mcp_servers", name: "MCP Servers", access_granted_via_app: true },
              },
            },
          },
        });
      }
      if (url === "/common/catalog/resources/7/agent/ast_11") {
        return Promise.resolve({
          data: {
            data: {
              type: "plugin_resource",
              id: "ast_11",
              attributes: {
                name: "Proxied Agent",
                kind: "7:agent",
                kind_label: "Agent",
                privacy_score: 20,
                catalogs: [],
                tags: [],
                access_granted_via_app: true,
                portal_detail_url: "/portal/plugins/asset-catalog#/assets/ast_11",
                resource_type: { plugin_id: 7, slug: "agent", name: "Agent", access_granted_via_app: false },
              },
            },
          },
        });
      }
      return Promise.reject(new Error(`unexpected ${url}`));
    });
  });

  it("shows models with prices, the allow list, privacy, base URL and catalogs", async () => {
    renderDetail("/portal/catalog/llms/7");
    expect(await screen.findByRole("heading", { level: 1, name: "Acme OpenAI" })).toBeInTheDocument();

    const models = screen.getByTestId("llm-models-section");
    expect(within(models).getByText("^gpt-4o-mini$")).toBeInTheDocument();
    const rows = within(models).getAllByTestId("llm-model-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("gpt-4o");
    expect(rows[0]).toHaveTextContent("Default");
    expect(rows[0]).toHaveTextContent("$2.50");
    expect(rows[0]).toHaveTextContent("$10.00");

    expect(screen.getByTestId("privacy-level-chip")).toHaveTextContent("Internal · 40");
    expect(screen.getByText("http://gw.example.com/ai/acme-openai/v1")).toBeInTheDocument();
    expect(within(screen.getByTestId("asset-catalogs")).getByText("Default")).toBeInTheDocument();
    expect(screen.getByText("region")).toBeInTheDocument();
    expect(screen.getByTestId("governed-metadata-badges")).toHaveTextContent("Owner: Platform team");
  });

  it("lists the caller's apps that already use the asset", async () => {
    renderDetail("/portal/catalog/llms/7");
    const list = await screen.findByTestId("asset-apps");
    expect(within(list).getByRole("link", { name: "Support bot" })).toHaveAttribute("href", "/portal/apps/1");
    expect(within(list).queryByText("Other app")).not.toBeInTheDocument();
    expect(within(list).getByTestId("app-status")).toHaveTextContent("Active");
  });

  it("says when an asset is outside the caller's visibility", async () => {
    renderDetail("/portal/catalog/llms/8");
    expect(await screen.findByText("This LLM provider is not available to you")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to browse" })).toHaveAttribute("href", "/portal/catalog/llms");
  });

  // An asset the providing plugin gates itself must not promise a credential:
  // no "Build app", a link to the plugin's page instead.
  it("sends a plugin-gated resource to the plugin's own page instead of the app builder", async () => {
    renderDetail("/portal/catalog/resources/7/agent/ast_9", "plugin_resource");
    expect(await screen.findByRole("heading", { level: 1, name: "Support Triage Agent" })).toBeInTheDocument();
    expect(screen.queryByTestId("asset-build-app")).not.toBeInTheDocument();
    expect(screen.getByTestId("asset-view-external")).toHaveTextContent("View in Agent");
    expect(screen.getByTestId("plugin-resource-access-note")).toHaveTextContent("managed by the plugin that provides them");
    expect(screen.getByRole("link", { name: "Open it there" })).toHaveAttribute("href", "/portal/plugins/asset-catalog#/assets/ast_9");
  });

  it("keeps Build app for an app-granted plugin resource", async () => {
    renderDetail("/portal/catalog/resources/3/mcp_servers/srv-1", "plugin_resource");
    expect(await screen.findByRole("heading", { level: 1, name: "Support MCP" })).toBeInTheDocument();
    expect(screen.getByTestId("asset-build-app")).toHaveTextContent("Build app");
    expect(screen.queryByTestId("asset-view-external")).not.toBeInTheDocument();
    expect(screen.getByTestId("plugin-resource-access-note")).toHaveTextContent("an app credential is what grants access");
  });

  // An app-granted resource whose plugin declared a page keeps Build app and
  // gains the plugin's page as a second view of the same item.
  it("offers the plugin's page next to Build app for an app-granted resource that has one", async () => {
    renderDetail("/portal/catalog/resources/7/agent/ast_11", "plugin_resource");
    expect(await screen.findByRole("heading", { level: 1, name: "Proxied Agent" })).toBeInTheDocument();
    expect(screen.getByTestId("asset-build-app")).toHaveTextContent("Build app");
    expect(screen.getByTestId("asset-view-external")).toHaveTextContent("View in Agent");
    const note = screen.getByTestId("plugin-resource-access-note");
    expect(note).toHaveTextContent("an app credential is what grants access");
    expect(note).toHaveTextContent("to see the full details");
    expect(screen.getByRole("link", { name: "Open it there" })).toHaveAttribute("href", "/portal/plugins/asset-catalog#/assets/ast_11");
  });

  it("shows tool operations and links to the API documentation", async () => {
    renderDetail("/portal/catalog/tools/4", "tool");
    expect(await screen.findByRole("heading", { level: 1, name: "Weather API" })).toBeInTheDocument();
    expect(screen.getByText("getForecast")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "API documentation" })).toHaveAttribute("href", "/portal/tools/4/docs");
    expect(screen.getByTestId("asset-build-app")).toHaveTextContent("Build app");
  });
});
