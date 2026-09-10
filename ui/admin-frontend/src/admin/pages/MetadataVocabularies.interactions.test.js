import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataVocabularies from "./MetadataVocabularies";
import {
  isGovernedMetadataAvailable,
  getMetadataVocabularies,
  createMetadataVocabulary,
  updateMetadataVocabulary,
  deleteMetadataVocabulary,
} from "../services/governedMetadataService";

jest.mock("../services/governedMetadataService", () => ({
  ...jest.requireActual("../services/governedMetadataService"),
  isGovernedMetadataAvailable: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  createMetadataVocabulary: jest.fn(),
  updateMetadataVocabulary: jest.fn(),
  deleteMetadataVocabulary: jest.fn(),
}));

const VOCAB = { id: 1, name: "Risk Tier", slug: "risk_tier", description: "how risky", source: "plugin:7",
  terms: [{ value: "low", label: "Low" }, { value: "old", label: "Old", deprecated: true }] };

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <MetadataVocabularies />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("MetadataVocabularies interactions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    isGovernedMetadataAvailable.mockResolvedValue(true);
    getMetadataVocabularies.mockResolvedValue([VOCAB]);
    updateMetadataVocabulary.mockResolvedValue({ id: 1 });
    deleteMetadataVocabulary.mockResolvedValue();
  });

  it("edits an existing vocabulary with its terms prefilled", async () => {
    renderPage();
    expect(await screen.findByText("Plugin 7")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Edit Risk Tier"));
    expect(await screen.findByText("Edit Vocabulary")).toBeInTheDocument();
    expect(screen.getByLabelText("Term 1 value")).toHaveValue("low");
    expect(screen.getByLabelText("Term 2 value")).toHaveValue("old");
    expect(screen.getByLabelText("Remove term 1")).not.toBeDisabled();

    fireEvent.click(screen.getByLabelText("Remove term 2"));
    expect(screen.getByLabelText("Remove term 1")).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Term 1 label"), { target: { value: "Lowest" } });
    fireEvent.click(screen.getByText("Update"));

    await waitFor(() => expect(updateMetadataVocabulary).toHaveBeenCalledWith(1, expect.objectContaining({
      name: "Risk Tier", slug: "risk_tier", description: "how risky",
      terms: [{ value: "low", label: "Lowest", description: "", deprecated: false }],
    })));
    expect(await screen.findByText("Vocabulary updated")).toBeInTheDocument();
  });

  it("deletes after confirmation and shows the server reason on failure", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Delete Risk Tier"));
    await screen.findByText("Delete Vocabulary");
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(deleteMetadataVocabulary).toHaveBeenCalledWith(1));
    expect(await screen.findByText("Vocabulary deleted")).toBeInTheDocument();

    deleteMetadataVocabulary.mockRejectedValueOnce(new Error("vocabulary is referenced by one or more schema fields"));
    fireEvent.click(await screen.findByLabelText("Delete Risk Tier"));
    await screen.findByText("Delete Vocabulary");
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(await screen.findByText(/referenced by one or more schema fields/)).toBeInTheDocument();
  });

  it("requires a name and reports a create failure", async () => {
    createMetadataVocabulary.mockRejectedValueOnce(new Error("slug already exists"));
    renderPage();
    await screen.findByText("Risk Tier");
    fireEvent.click(screen.getByText("Add Vocabulary"));
    fireEvent.change(screen.getByLabelText("Term 1 value"), { target: { value: "x" } });
    fireEvent.click(screen.getByText("Create"));
    expect(await screen.findByText("Name is required")).toBeInTheDocument();
    expect(createMetadataVocabulary).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Dup" } });
    fireEvent.click(screen.getByText("Create"));
    expect(await screen.findByText("slug already exists")).toBeInTheDocument();
  });

  it("shows the enterprise badge and a load error", async () => {
    isGovernedMetadataAvailable.mockResolvedValueOnce(false);
    renderPage();
    expect(await screen.findByText(/Enterprise Edition/)).toBeInTheDocument();
    expect(getMetadataVocabularies).not.toHaveBeenCalled();

    getMetadataVocabularies.mockRejectedValueOnce(new Error("boom"));
    renderPage();
    expect(await screen.findByText("Failed to load vocabularies")).toBeInTheDocument();
  });
});
