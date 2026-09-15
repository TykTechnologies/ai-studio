import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import ToolForm from "./ToolForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import { usePermissions } from "../../context/PermissionsContext";
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
} from "../../services/governedMetadataService";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: jest.fn(),
}));
jest.mock("../../hooks/useSystemFeatures", () => ({ __esModule: true, default: () => ({ features: {} }) }));
jest.mock("../../services/governedMetadataService", () => ({
  ...jest.requireActual("../../services/governedMetadataService"),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
  validateObjectMetadata: jest.fn(),
}));
jest.mock("../../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});
jest.mock("../common/relationship-picker", () =>
  require("../../../test-utils/component-mocks").relationshipPickerMock,
);

const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const definition = {
  parameters: { type: "object", properties: { action: { type: "string" } }, required: ["action"] },
  ui: { kind: "form", title: "Confirm the action", response_schema: { type: "object", properties: { ok: { type: "boolean" } } } },
};

const clientToolPayload = () => ({
  data: {
    data: {
      id: "9",
      governed_metadata: {},
      attributes: {
        name: "Ask approval", description: "Asks the user", tool_type: "CLIENT",
        oas_spec: btoa(JSON.stringify(definition)),
        privacy_score: 10, active: true, operations: [], file_stores: [], filters: [], namespace: "",
      },
    },
  },
});

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <ToolForm />
      </MemoryRouter>
    </ThemeProvider>,
  );

const decodeSpec = (b64) => JSON.parse(decodeURIComponent(escape(atob(b64))));
const lastPostAttributes = () => apiClient.post.mock.calls.find((c) => c[0] === "/tools")[1].data.attributes;

describe("ToolForm client (human-in-the-loop) tools", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    usePermissions.mockReturnValue({ can: () => true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/tools") return Promise.resolve({ data: { data: [] } });
      if (url === "/tools/9") return Promise.resolve(clientToolPayload());
      if (url === "/tools/9/operations") return Promise.resolve({ data: { data: { operations: [] } } });
      if (url === "/tools/9/filters") return Promise.resolve({ data: { data: [] } });
      if (url === "/tools/9/dependencies") return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.post.mockResolvedValue({ data: { data: { id: "11" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "9" } } });
    apiClient.delete.mockResolvedValue({ data: {} });
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("creates a client tool with its definition encoded in the spec field", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Ask approval" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Asks the user" } });
    fireEvent.click(screen.getByLabelText("Client (human-in-the-loop)"));

    // REST-only sections disappear, the field builder and preview appear.
    expect(screen.queryByLabelText("OAS Spec")).not.toBeInTheDocument();
    expect(screen.getByTestId("client-parameters-builder")).toBeInTheDocument();
    expect(screen.getByTestId("client-tool-preview")).toBeInTheDocument();

    // The default definition has one "summary" detail; rename it to "Action".
    fireEvent.change(screen.getByLabelText("Detail 1 label"), { target: { value: "Action" } });
    expect(screen.getByLabelText("Detail 1 identifier").value).toBe("action");
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));

    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    const attrs = lastPostAttributes();
    expect(attrs.tool_type).toBe("CLIENT");
    expect(attrs.operations).toEqual([]);
    const spec = decodeSpec(attrs.oas_spec);
    expect(spec.ui).toEqual({ kind: "approval" });
    expect(spec.parameters.required).toEqual(["action"]);
    expect(spec.parameters.properties.action).toMatchObject({ type: "string", title: "Action" });
  });

  it("pre-fills everything from a shipped example", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });
    fireEvent.click(screen.getByLabelText("Client (human-in-the-loop)"));

    fireEvent.mouseDown(screen.getByLabelText(/Interaction/));
    fireEvent.click(await screen.findByRole("option", { name: "Form" }));
    fireEvent.click(screen.getByText("Shipping address (US)"));

    // Name and description come from the preset; the response builder shows its fields.
    expect(screen.getByLabelText(/^Name/).value).toBe("Shipping address (US)");
    expect(screen.getByLabelText(/^Description/).value).toMatch(/shipping address/i);
    expect(screen.getByTestId("client-response-builder")).toBeInTheDocument();
    expect(screen.getAllByTestId("client-response-row").length).toBeGreaterThan(5);
    // The preview renders the form with the preset's fields.
    expect(screen.getByTestId("client-tool-preview-card")).toHaveTextContent("Where should we ship to?");

    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    const spec = decodeSpec(lastPostAttributes().oas_spec);
    expect(spec.ui.kind).toBe("form");
    expect(spec.ui.title).toBe("Where should we ship to?");
    expect(spec.ui.response_schema.required).toEqual(expect.arrayContaining(["full_name", "city", "state", "zip"]));
    expect(spec.ui.response_schema.properties.state.enum).toContain("CA");
    expect(spec.parameters.properties.purpose).toBeDefined();
  });

  it("creates a generative UI (present) tool without a hand-written schema", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "present" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Draw UI" } });
    fireEvent.click(screen.getByLabelText("Client (human-in-the-loop)"));

    fireEvent.mouseDown(screen.getByLabelText(/Interaction/));
    fireEvent.click(await screen.findByRole("option", { name: "Generative UI (present)" }));

    // The schema editor is replaced by a note: the vocabulary is built in.
    expect(screen.queryByTestId("client-parameters")).not.toBeInTheDocument();
    expect(screen.getByTestId("present-schema-note")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    expect(decodeSpec(lastPostAttributes().oas_spec)).toEqual({ ui: { kind: "present" } });
  });

  it("refuses an invalid parameters schema", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Ask" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Asks" } });
    fireEvent.click(screen.getByLabelText("Client (human-in-the-loop)"));
    fireEvent.click(screen.getByTestId("client-parameters-toggle-json"));
    fireEvent.change(screen.getByTestId("client-parameters-json"), { target: { value: "{not json" } });
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    expect(await screen.findByText(/Parameters schema is not valid JSON/)).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalledWith("/tools", expect.anything());
  });

  it("loads an existing client tool's definition into the editor", async () => {
    mockParams = { id: "9" };
    renderForm();
    await screen.findByDisplayValue("Ask approval");
    expect(screen.getByLabelText("Client (human-in-the-loop)")).toBeChecked();
    expect(screen.getByDisplayValue("Confirm the action")).toBeInTheDocument();
    // The stored schemas are shown as fields: one parameter, one response field.
    expect(screen.getByLabelText("Detail 1 identifier").value).toBe("action");
    expect(screen.getByTestId("client-response-builder")).toBeInTheDocument();
    const responseRows = screen.getAllByTestId("client-response-row");
    expect(responseRows).toHaveLength(1);
    expect(responseRows[0]).toHaveTextContent("Yes / no");
  });
});
