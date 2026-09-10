import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataCompliance from "./MetadataCompliance";
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

describe("MetadataCompliance interactions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    isGovernedMetadataAvailable.mockResolvedValue(true);
    getMetadataObjectTypes.mockResolvedValue([
      { slug: "llm", label: "LLM", source: "builtin" },
      { slug: "plugin_resource:7:w", label: "Widgets", source: "plugin:7" },
    ]);
    getMetadataComplianceReport.mockResolvedValue({ counts: {}, entries: [] });
  });

  it("refetches with the chosen filters and hides plugin types from the type filter", async () => {
    renderPage();
    expect(await screen.findByText(/Nothing to report/)).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByLabelText(/Object type/));
    expect(await screen.findByRole("option", { name: "LLM" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Widgets" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: "LLM" }));
    await waitFor(() => expect(getMetadataComplianceReport).toHaveBeenLastCalledWith({ object_type: "llm" }));

    fireEvent.mouseDown(screen.getByLabelText(/^Status/));
    fireEvent.click(await screen.findByRole("option", { name: "Expired" }));
    await waitFor(() => expect(getMetadataComplianceReport).toHaveBeenLastCalledWith({ object_type: "llm", status: "expired" }));
  });

  it("renders Never for unvalidated objects and no edit action for plugin objects", async () => {
    getMetadataComplianceReport.mockResolvedValue({
      counts: { missing: 1, expired: 1 },
      entries: [
        { object_type: "llm", object_id: "1", object_name: "gpt", status: "missing", issues: [] },
        { object_type: "plugin_resource:7:w", object_id: "abc", object_name: "widget-a", status: "expired", issues: [{ field: "exp", message: "past" }], last_validated_at: "2026-01-01T00:00:00Z" },
      ],
    });
    renderPage();
    expect(await screen.findByText("gpt")).toBeInTheDocument();
    expect(screen.getByText("Never")).toBeInTheDocument();
    expect(screen.getByText("widget-a").closest("a")).toBeNull();
    expect(screen.getAllByText("Edit")).toHaveLength(1);
    expect(screen.getByText("Widgets")).toBeInTheDocument();
    expect(screen.getAllByText("Expired").length).toBeGreaterThanOrEqual(2); // count tile + row chip
  });

  it("shows an error when the report fails", async () => {
    getMetadataComplianceReport.mockRejectedValueOnce(new Error("boom"));
    renderPage();
    expect(await screen.findByText(/Failed to load the metadata coverage report/)).toBeInTheDocument();
  });
});
