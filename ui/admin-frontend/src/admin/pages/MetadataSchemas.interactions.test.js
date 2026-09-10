import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataSchemas from "./MetadataSchemas";
import {
  isGovernedMetadataAvailable,
  getMetadataSchemas,
  getMetadataObjectTypes,
  deleteMetadataSchema,
} from "../services/governedMetadataService";

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));
jest.mock("../services/governedMetadataService", () => ({
  ...jest.requireActual("../services/governedMetadataService"),
  isGovernedMetadataAvailable: jest.fn(),
  getMetadataSchemas: jest.fn(),
  getMetadataObjectTypes: jest.fn(),
  deleteMetadataSchema: jest.fn(),
}));

const SCHEMAS = [
  { id: 1, name: "Core", slug: "core", applies_to: ["*"], fields: [], enforcement: "advisory", active: true, source: "admin" },
];

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <MetadataSchemas />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("MetadataSchemas interactions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    isGovernedMetadataAvailable.mockResolvedValue(true);
    getMetadataObjectTypes.mockResolvedValue([]);
    getMetadataSchemas.mockResolvedValue(SCHEMAS);
    deleteMetadataSchema.mockResolvedValue();
  });

  it("navigates to the create and edit forms", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Schema"));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/metadata/schemas/new");
    fireEvent.click(await screen.findByLabelText("Edit Core"));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/metadata/schemas/edit/1");
  });

  it("deletes after confirmation and refreshes the list", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Delete Core"));
    expect(await screen.findByText("Delete Schema")).toBeInTheDocument();
    getMetadataSchemas.mockResolvedValueOnce([]);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(deleteMetadataSchema).toHaveBeenCalledWith(1));
    expect(await screen.findByText("Schema deleted")).toBeInTheDocument();
    expect(await screen.findByText(/No metadata schemas yet/)).toBeInTheDocument();
  });

  it("cancelling the confirmation deletes nothing", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Delete Core"));
    await screen.findByText("Delete Schema");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByText("Delete Schema")).not.toBeInTheDocument());
    expect(deleteMetadataSchema).not.toHaveBeenCalled();
  });

  it("surfaces a delete failure", async () => {
    deleteMetadataSchema.mockRejectedValueOnce(new Error("plugin-sourced schemas can only have active and enforcement changed"));
    renderPage();
    fireEvent.click(await screen.findByLabelText("Delete Core"));
    await screen.findByText("Delete Schema");
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(await screen.findByText(/plugin-sourced schemas can only/)).toBeInTheDocument();
  });

  it("shows an error when loading fails", async () => {
    getMetadataSchemas.mockRejectedValueOnce(new Error("boom"));
    renderPage();
    expect(await screen.findByText("Failed to load metadata schemas")).toBeInTheDocument();
  });
});
