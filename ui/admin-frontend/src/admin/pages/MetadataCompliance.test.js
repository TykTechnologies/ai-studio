import React from "react";
import { render, screen, within, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataCompliance, { objectEditPath, objectViewPath, openDetailPath } from "./MetadataCompliance";
import {
  isGovernedMetadataAvailable,
  getMetadataComplianceReport,
  getMetadataObjectTypes,
} from "../services/governedMetadataService";

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

jest.mock("../services/governedMetadataService", () => ({
  ...jest.requireActual("../services/governedMetadataService"),
  isGovernedMetadataAvailable: jest.fn(),
  getMetadataComplianceReport: jest.fn(),
  getMetadataObjectTypes: jest.fn(),
}));

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <MetadataCompliance />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("MetadataCompliance", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    isGovernedMetadataAvailable.mockResolvedValue(true);
    getMetadataObjectTypes.mockResolvedValue([{ slug: "llm", label: "LLM", source: "builtin" }]);
    getMetadataComplianceReport.mockResolvedValue({
      counts: { missing: 1, valid: 1 },
      entries: [
        { object_type: "llm", object_id: "7", object_name: "gpt-4", status: "missing", issues: [{ field: "risk_tier", code: "required", message: "Risk tier is required" }] },
        { object_type: "llm", object_id: "8", object_name: "claude", status: "valid", issues: [], last_validated_at: "2026-09-01T00:00:00Z" },
      ],
    });
  });

  it("renders counts, rows, issues and object links", async () => {
    renderPage();
    expect(await screen.findByText("gpt-4")).toBeInTheDocument();
    expect(screen.getByText("gpt-4").closest("a")).toHaveAttribute("href", "/admin/llms/7");
    expect(screen.getByText(/Risk tier is required/)).toBeInTheDocument();
    expect(screen.getByText("claude")).toBeInTheDocument();
    expect(screen.getAllByText("Edit")[0].closest("a")).toHaveAttribute("href", "/admin/llms/edit/7");
    expect(getMetadataComplianceReport).toHaveBeenCalledWith({});
  });

  it("shows the enterprise badge when unavailable", async () => {
    isGovernedMetadataAvailable.mockResolvedValue(false);
    renderPage();
    expect(await screen.findByText(/Enterprise Edition/)).toBeInTheDocument();
    expect(getMetadataComplianceReport).not.toHaveBeenCalled();
  });

  it("maps object types to routes", () => {
    expect(objectEditPath("tool", "3")).toBe("/admin/tools/edit/3");
    expect(objectViewPath("datasource", "4")).toBe("/admin/datasources/4");
    expect(objectEditPath("plugin_resource:7:widgets", "x")).toBeNull();
    // MCP servers have a detail page but no edit route.
    expect(objectViewPath("mcp_server", "9")).toBe("/admin/mcp-servers/9");
    expect(objectEditPath("mcp_server", "9")).toBe("/admin/mcp-servers/9");
  });

  describe("plugin resource rows", () => {
    beforeEach(() => {
      getMetadataObjectTypes.mockResolvedValue([
        { slug: "llm", label: "LLM", source: "builtin" },
        { slug: "plugin_resource:7:assets", label: "Catalog assets", source: "plugin" },
      ]);
      getMetadataComplianceReport.mockResolvedValue({
        counts: { missing: 2 },
        entries: [
          { object_type: "llm", object_id: "7", object_name: "gpt-4", status: "missing", issues: [] },
          {
            object_type: "plugin_resource:7:assets",
            object_id: "ast_1",
            object_name: "Welcome prompt",
            status: "missing",
            issues: [],
            detail_path: "/portal/plugins/asset-catalog#/assets/ast_1",
          },
          { object_type: "plugin_resource:7:assets", object_id: "ast_2", object_name: "No path", status: "missing", issues: [] },
        ],
      });
    });

    it("offers Open for a row with a detail path and Edit for a built-in row", async () => {
      renderPage();
      const pluginRow = (await screen.findByText("Welcome prompt")).closest("tr");
      const open = within(pluginRow).getByRole("button", { name: "Open" });
      expect(within(pluginRow).queryByText("Edit")).not.toBeInTheDocument();
      fireEvent.click(open);
      expect(mockNavigate).toHaveBeenCalledWith("/portal/plugins/asset-catalog#/assets/ast_1");

      const llmRow = screen.getByText("gpt-4").closest("tr");
      expect(within(llmRow).getByText("Edit").closest("a")).toHaveAttribute("href", "/admin/llms/edit/7");
      expect(within(llmRow).queryByRole("button", { name: "Open" })).not.toBeInTheDocument();

      // A plugin row without a path gets neither.
      const bareRow = screen.getByText("No path").closest("tr");
      expect(within(bareRow).queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
      expect(within(bareRow).queryByText("Edit")).not.toBeInTheDocument();
    });
  });
});

describe("openDetailPath", () => {
  afterEach(() => {
    window.history.replaceState({}, "", "/");
  });

  it("uses the router for in-app paths", () => {
    const navigate = jest.fn();
    openDetailPath("/portal/plugins/asset-catalog#/assets/ast_1", navigate);
    expect(navigate).toHaveBeenCalledWith("/portal/plugins/asset-catalog#/assets/ast_1");
  });

  it("also fires popstate when only the hash of the current page changes", () => {
    // react-router's navigate alone would leave a plugin web component on its
    // old sub-route: it re-reads the hash on popstate.
    window.history.replaceState({}, "", "/portal/plugins/asset-catalog#/assets/ast_0");
    const navigate = jest.fn();
    const onPop = jest.fn();
    window.addEventListener("popstate", onPop);
    try {
      openDetailPath("/portal/plugins/asset-catalog#/assets/ast_1", navigate);
      expect(navigate).toHaveBeenCalledWith("/portal/plugins/asset-catalog#/assets/ast_1");
      expect(onPop).toHaveBeenCalledTimes(1);
    } finally {
      window.removeEventListener("popstate", onPop);
    }
  });

  it("does a full load for anything that is not an in-app path", () => {
    const navigate = jest.fn();
    const assign = jest.fn();
    const original = window.location;
    delete window.location;
    window.location = { ...original, assign };
    try {
      openDetailPath("https://docs.example.com/x", navigate);
      expect(navigate).not.toHaveBeenCalled();
      expect(assign).toHaveBeenCalledWith("https://docs.example.com/x");
    } finally {
      window.location = original;
    }
  });
});
