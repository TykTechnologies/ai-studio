import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../../admin/utils/testTheme";
import AssetCard from "./AssetCard";

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const renderCard = (item) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <AssetCard item={item} />
      </MemoryRouter>
    </ThemeProvider>,
  );

const base = { name: "Thing", short_description: "", kind: "x", privacy_score: 10, tags: [], catalogs: [] };

// The primary action promises a credential. Only items an app credential
// actually unlocks get it; the rest point at the plugin that gates them.
describe("AssetCard primary action", () => {
  beforeEach(() => {
    mockNavigate.mockClear();
  });

  it("offers Build app for an LLM provider", () => {
    renderCard({ type: "llm", id: "1", attributes: { ...base, name: "Acme OpenAI" } });
    fireEvent.click(screen.getByTestId("asset-card-build"));
    expect(mockNavigate).toHaveBeenCalledWith("/portal/app/new?llm=1");
    expect(screen.queryByTestId("asset-card-view")).not.toBeInTheDocument();
  });

  it("offers Build app for an app-granted plugin resource", () => {
    renderCard({
      type: "plugin_resource",
      id: "srv-1",
      attributes: { ...base, name: "Support MCP", access_granted_via_app: true, resource_type: { plugin_id: 3, slug: "mcp_servers", name: "MCP Servers" } },
    });
    expect(screen.getByTestId("asset-card-build")).toHaveTextContent("Build app");
  });

  it("links a plugin-gated resource to the plugin's page instead", () => {
    renderCard({
      type: "plugin_resource",
      id: "ast_9",
      attributes: {
        ...base,
        name: "Support Agent",
        access_granted_via_app: false,
        portal_detail_url: "/portal/plugins/asset-catalog#/assets/ast_9",
        resource_type: { plugin_id: 7, slug: "agent", name: "Agent" },
      },
    });
    expect(screen.queryByTestId("asset-card-build")).not.toBeInTheDocument();
    const view = screen.getByTestId("asset-card-view");
    expect(view).toHaveTextContent("View in Agent");
    fireEvent.click(view);
    expect(mockNavigate).toHaveBeenCalledWith("/portal/plugins/asset-catalog#/assets/ast_9");
  });

  it("shows only Details when there is nowhere else to go", () => {
    renderCard({
      type: "plugin_resource",
      id: "ast_10",
      attributes: { ...base, name: "Note", access_granted_via_app: false, resource_type: { plugin_id: 7, slug: "prompt", name: "Prompt" } },
    });
    expect(screen.queryByTestId("asset-card-build")).not.toBeInTheDocument();
    expect(screen.queryByTestId("asset-card-view")).not.toBeInTheDocument();
    expect(screen.getByTestId("asset-card-details")).toBeInTheDocument();
  });
});
