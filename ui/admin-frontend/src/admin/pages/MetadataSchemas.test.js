import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataSchemas from "./MetadataSchemas";
import {
  isGovernedMetadataAvailable,
  getMetadataSchemas,
  getMetadataObjectTypes,
} from "../services/governedMetadataService";

jest.mock("../services/governedMetadataService", () => ({
  ...jest.requireActual("../services/governedMetadataService"),
  isGovernedMetadataAvailable: jest.fn(),
  getMetadataSchemas: jest.fn(),
  getMetadataObjectTypes: jest.fn(),
  deleteMetadataSchema: jest.fn(),
}));

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <MetadataSchemas />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("MetadataSchemas", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    isGovernedMetadataAvailable.mockResolvedValue(true);
    getMetadataObjectTypes.mockResolvedValue([{ slug: "llm", label: "LLM", source: "builtin" }]);
    getMetadataSchemas.mockResolvedValue([]);
  });

  it("shows the enterprise badge when unavailable", async () => {
    isGovernedMetadataAvailable.mockResolvedValue(false);
    renderPage();
    expect(await screen.findByText(/Enterprise Edition/)).toBeInTheDocument();
    expect(screen.queryByText("Add Schema")).not.toBeInTheDocument();
    expect(getMetadataSchemas).not.toHaveBeenCalled();
  });

  it("renders the empty state", async () => {
    renderPage();
    expect(await screen.findByText(/No metadata schemas yet/)).toBeInTheDocument();
    expect(screen.getByText("Add Schema")).toBeInTheDocument();
  });

  it("renders rows with enforcement, source and applies-to labels", async () => {
    getMetadataSchemas.mockResolvedValue([
      { id: 1, name: "Core", slug: "governance-core", applies_to: ["*"], fields: [{ key: "a" }], enforcement: "enforce", active: true, source: "admin" },
      { id: 2, name: "Widgets", slug: "widgets", applies_to: ["llm"], fields: [], enforcement: "advisory", active: false, source: "plugin:7" },
    ]);
    renderPage();
    expect(await screen.findByText("Core")).toBeInTheDocument();
    expect(screen.getByText("Enforced")).toBeInTheDocument();
    expect(screen.getByText("Advisory")).toBeInTheDocument();
    expect(screen.getByText("All object types")).toBeInTheDocument();
    expect(screen.getByText("LLM")).toBeInTheDocument();
    expect(screen.getByText("Plugin 7")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText("Delete Widgets")).toBeDisabled());
  });
});
