import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MetadataSchemaForm from "./MetadataSchemaForm";
import {
  getMetadataSchema,
  createMetadataSchema,
  updateMetadataSchema,
  getMetadataObjectTypes,
  getMetadataVocabularies,
} from "../services/governedMetadataService";

const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));
jest.mock("../services/governedMetadataService", () => ({
  ...jest.requireActual("../services/governedMetadataService"),
  getMetadataSchema: jest.fn(),
  createMetadataSchema: jest.fn(),
  updateMetadataSchema: jest.fn(),
  getMetadataObjectTypes: jest.fn(),
  getMetadataVocabularies: jest.fn(),
}));

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <MetadataSchemaForm />
      </MemoryRouter>
    </ThemeProvider>
  );

const EXISTING = {
  id: 3, name: "Core", slug: "core", source: "admin", applies_to: ["*"], enforcement: "advisory", active: true, description: "d",
  fields: [
    { key: "one", label: "One", type: "string", order: 1 },
    { key: "two", label: "Two", type: "number", order: 2 },
    { key: "three", label: "Three", type: "boolean", order: 3 },
  ],
};

describe("MetadataSchemaForm interactions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "3" };
    getMetadataObjectTypes.mockResolvedValue([
      { slug: "llm", label: "LLM", source: "builtin" },
      { slug: "tool", label: "Tool", source: "builtin" },
    ]);
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataSchema.mockResolvedValue(EXISTING);
    updateMetadataSchema.mockResolvedValue({ id: 3 });
    createMetadataSchema.mockResolvedValue({ id: 9 });
  });

  it("reorders, edits and removes fields, then saves the new order", async () => {
    renderForm();
    await screen.findByText("Edit Metadata Schema");
    expect(screen.getByLabelText("Move one up")).toBeDisabled();
    expect(screen.getByLabelText("Move three down")).toBeDisabled();

    fireEvent.click(screen.getByLabelText("Move three up"));
    fireEvent.click(screen.getByLabelText("Delete one"));

    fireEvent.click(screen.getByLabelText("Edit two"));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/^Label/), { target: { value: "Second" } });
    fireEvent.click(within(dialog).getByText("Update field"));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    fireEvent.click(screen.getByText("Save schema"));
    await waitFor(() => expect(updateMetadataSchema).toHaveBeenCalledTimes(1));
    const [id, payload] = updateMetadataSchema.mock.calls[0];
    expect(id).toBe("3");
    expect(payload.fields.map((f) => [f.key, f.order])).toEqual([["three", 1], ["two", 2]]);
    expect(payload.fields[1].label).toBe("Second");
    expect(payload.description).toBe("d");
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/metadata/schemas"), { timeout: 3000 });
  });

  it("choosing All object types clears specific ones and vice versa", async () => {
    mockParams = {};
    renderForm();
    await screen.findByText("New Metadata Schema");
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "X" } });

    fireEvent.mouseDown(screen.getByLabelText(/Applies to/));
    fireEvent.click(await screen.findByRole("option", { name: "LLM" }));
    fireEvent.click(screen.getByRole("option", { name: "Tool" }));
    fireEvent.click(screen.getByRole("option", { name: "All object types" }));
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "Escape" });

    fireEvent.click(screen.getByText("Create schema"));
    await waitFor(() => expect(createMetadataSchema).toHaveBeenCalled());
    expect(createMetadataSchema.mock.calls[0][0].applies_to).toEqual(["*"]);
  });

  it("warns about enforcement and reports a save failure without navigating", async () => {
    updateMetadataSchema.mockRejectedValueOnce(new Error('field key "one" collides with schema "other"'));
    renderForm();
    await screen.findByText("Edit Metadata Schema");
    fireEvent.mouseDown(screen.getByLabelText(/Enforcement/));
    fireEvent.click(await screen.findByRole("option", { name: /Enforce: block/ }));
    expect(screen.getByText(/Check the compliance report before switching this on/)).toBeInTheDocument();

    fireEvent.click(screen.getByText("Save schema"));
    expect(await screen.findByText(/collides with schema/)).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(updateMetadataSchema.mock.calls[0][1].enforcement).toBe("enforce");
  });

  it("refuses to submit without a name or object type", async () => {
    mockParams = {};
    renderForm();
    await screen.findByText("New Metadata Schema");
    fireEvent.click(screen.getByText("Create schema"));
    expect(await screen.findByText("Name is required")).toBeInTheDocument();
    expect(createMetadataSchema).not.toHaveBeenCalled();
  });

  it("keeps a manually edited slug and shows a load error", async () => {
    mockParams = {};
    renderForm();
    await screen.findByText("New Metadata Schema");
    fireEvent.change(screen.getByLabelText(/^Slug/), { target: { value: "custom" } });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Something Else" } });
    expect(screen.getByLabelText(/^Slug/)).toHaveValue("custom");

    mockParams = { id: "77" };
    getMetadataSchema.mockRejectedValueOnce(new Error("not found"));
    renderForm();
    expect(await screen.findByText("not found")).toBeInTheDocument();
  });

  it("links back to the schema list", async () => {
    renderForm();
    await screen.findByText("Edit Metadata Schema");
    expect(screen.getByLabelText("Back to schemas").closest("a")).toHaveAttribute("href", "/admin/metadata/schemas");
    expect(screen.getByText("Cancel").closest("a")).toHaveAttribute("href", "/admin/metadata/schemas");
  });
});
