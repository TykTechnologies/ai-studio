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
} from "../services/governedMetadataService";

jest.mock("../services/governedMetadataService", () => ({
  ...jest.requireActual("../services/governedMetadataService"),
  isGovernedMetadataAvailable: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  createMetadataVocabulary: jest.fn(),
  updateMetadataVocabulary: jest.fn(),
  deleteMetadataVocabulary: jest.fn(),
}));

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <MetadataVocabularies />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("MetadataVocabularies", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    isGovernedMetadataAvailable.mockResolvedValue(true);
    getMetadataVocabularies.mockResolvedValue([
      { id: 1, name: "Risk Tier", slug: "risk_tier", source: "admin", terms: [{ value: "low", label: "Low" }, { value: "high", label: "High" }] },
    ]);
    createMetadataVocabulary.mockResolvedValue({ id: 2 });
  });

  it("lists vocabularies with their terms", async () => {
    renderPage();
    expect(await screen.findByText("Risk Tier")).toBeInTheDocument();
    expect(screen.getByText("Low")).toBeInTheDocument();
    expect(screen.getByText("High")).toBeInTheDocument();
  });

  it("creates a vocabulary from the dialog", async () => {
    renderPage();
    await screen.findByText("Risk Tier");
    fireEvent.click(screen.getByText("Add Vocabulary"));
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Lifecycle" } });
    fireEvent.change(screen.getByLabelText("Term 1 value"), { target: { value: "draft" } });
    fireEvent.change(screen.getByLabelText("Term 1 label"), { target: { value: "Draft" } });
    fireEvent.click(screen.getByText("Add term"));
    fireEvent.change(screen.getByLabelText("Term 2 value"), { target: { value: "active" } });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(createMetadataVocabulary).toHaveBeenCalledTimes(1));
    expect(createMetadataVocabulary.mock.calls[0][0]).toEqual({
      name: "Lifecycle",
      slug: "",
      description: "",
      terms: [
        { value: "draft", label: "Draft", description: "", deprecated: false },
        { value: "active", label: "active", description: "", deprecated: false },
      ],
    });
  });

  it("refuses a vocabulary without terms", async () => {
    renderPage();
    await screen.findByText("Risk Tier");
    fireEvent.click(screen.getByText("Add Vocabulary"));
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Empty" } });
    fireEvent.click(screen.getByText("Create"));
    expect(await screen.findByText("At least one term is required")).toBeInTheDocument();
    expect(createMetadataVocabulary).not.toHaveBeenCalled();
  });
});
