import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataCompliance, { objectEditPath, objectViewPath } from "./MetadataCompliance";
import {
  isGovernedMetadataAvailable,
  getMetadataComplianceReport,
  getMetadataObjectTypes,
} from "../services/governedMetadataService";

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
  });
});
