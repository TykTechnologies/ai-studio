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

describe("MetadataSchemaForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    getMetadataObjectTypes.mockResolvedValue([{ slug: "llm", label: "LLM", source: "builtin" }]);
    getMetadataVocabularies.mockResolvedValue([{ slug: "risk_tier", name: "Risk" }]);
    createMetadataSchema.mockResolvedValue({ id: 1 });
    updateMetadataSchema.mockResolvedValue({ id: 1 });
  });

  it("creates a schema with an auto slug and ordered fields", async () => {
    renderForm();
    await screen.findByText("New Metadata Schema");

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Risk Register" } });
    expect(screen.getByLabelText(/^Slug/)).toHaveValue("risk-register");

    fireEvent.click(screen.getByText("Add field"));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/^Key/), { target: { value: "Bad Key" } });
    fireEvent.click(within(dialog).getByText("Add field"));
    expect(await within(dialog).findByText(/lowercase letters/)).toBeInTheDocument();

    fireEvent.change(within(dialog).getByLabelText(/^Key/), { target: { value: "risk_tier" } });
    fireEvent.change(within(dialog).getByLabelText(/^Label/), { target: { value: "Risk tier" } });
    fireEvent.click(within(dialog).getByText("Add field"));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByText("risk_tier")).toBeInTheDocument();

    // Duplicate key is refused in the dialog.
    fireEvent.click(screen.getByText("Add field"));
    const dialog2 = await screen.findByRole("dialog");
    fireEvent.change(within(dialog2).getByLabelText(/^Key/), { target: { value: "risk_tier" } });
    fireEvent.click(within(dialog2).getByText("Add field"));
    expect(await within(dialog2).findByText(/already exists/)).toBeInTheDocument();
    fireEvent.click(within(dialog2).getByText("Cancel"));

    fireEvent.click(screen.getByText("Create schema"));
    await waitFor(() => expect(createMetadataSchema).toHaveBeenCalledTimes(1));
    const payload = createMetadataSchema.mock.calls[0][0];
    expect(payload.name).toBe("Risk Register");
    expect(payload.slug).toBe("risk-register");
    expect(payload.applies_to).toEqual(["*"]);
    expect(payload.enforcement).toBe("advisory");
    expect(payload.fields).toHaveLength(1);
    expect(payload.fields[0]).toMatchObject({ key: "risk_tier", label: "Risk tier", type: "string", order: 1 });
  });

  it("loads an existing schema and disables structure for plugin-sourced ones", async () => {
    mockParams = { id: "5" };
    getMetadataSchema.mockResolvedValue({
      id: 5, name: "Widgets", slug: "widgets", source: "plugin:7", applies_to: ["llm"], enforcement: "advisory", active: false,
      fields: [{ key: "kind", label: "Kind", type: "string", order: 1 }],
    });
    renderForm();
    await screen.findByText("Edit Metadata Schema");
    expect(getMetadataSchema).toHaveBeenCalledWith("5");
    expect(screen.getByText(/contributed by a plugin/)).toBeInTheDocument();
    expect(screen.getByLabelText(/^Name/)).toBeDisabled();
    expect(screen.queryByText("Add field")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Save schema"));
    await waitFor(() => expect(updateMetadataSchema).toHaveBeenCalledWith("5", expect.objectContaining({ slug: "widgets", fields: [expect.objectContaining({ key: "kind" })] })));
  });
});
