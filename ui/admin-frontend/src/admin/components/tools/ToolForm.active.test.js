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

const toolPayload = (active) => ({
  data: {
    data: {
      id: "7",
      governed_metadata: {},
      attributes: {
        name: "weather", description: "Forecasts", tool_type: "REST", oas_spec: "",
        privacy_score: 40, active, operations: [], file_stores: [], filters: [], namespace: "",
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

const lastPatchAttributes = () => apiClient.patch.mock.calls[0][1].data.attributes;
const lastPostAttributes = () => apiClient.post.mock.calls.find((c) => c[0] === "/tools")[1].data.attributes;

describe("ToolForm Active switch and privacy level", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    usePermissions.mockReturnValue({ can: () => true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/tools") return Promise.resolve({ data: { data: [] } });
      if (url === "/tools/7") return Promise.resolve(toolPayload(true));
      if (url === "/tools/7/operations") return Promise.resolve({ data: { data: { operations: [] } } });
      if (url === "/tools/7/filters") return Promise.resolve({ data: { data: [] } });
      if (url === "/tools/7/dependencies") return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.post.mockResolvedValue({ data: { data: { id: "11" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "7" } } });
    apiClient.delete.mockResolvedValue({ data: {} });
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("shows the Active switch with its caption and the named privacy level", async () => {
    mockParams = { id: "7" };
    renderForm();
    await screen.findByDisplayValue("weather");
    expect(screen.getByRole("checkbox", { name: "Active" })).toBeChecked();
    expect(screen.getByText("Available to the portal and gateway when on")).toBeInTheDocument();
    // 40 sits in the Internal band.
    expect(screen.getByTestId("privacy-level-select")).toHaveValue("internal");
    expect(screen.getByRole("spinbutton", { name: "Privacy level score" })).toHaveValue(40);
  });

  it("PATCHes active=false when the switch is turned off on an existing tool", async () => {
    mockParams = { id: "7" };
    renderForm();
    await screen.findByDisplayValue("weather");
    fireEvent.click(screen.getByRole("checkbox", { name: "Active" }));
    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/tools/7", expect.anything()));
    expect(lastPatchAttributes().active).toBe(false);
    expect(lastPatchAttributes().privacy_score).toBe(40);
  });

  it("creates a new tool live when the user may publish", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });
    expect(screen.getByRole("checkbox", { name: "Active" })).toBeChecked();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Time" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Clock" } });
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    expect(lastPostAttributes().active).toBe(true);
  });

  it("starts a new tool off, with the switch locked, when the user cannot publish", async () => {
    usePermissions.mockReturnValue({ can: () => false });
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });
    const toggle = screen.getByRole("checkbox", { name: "Active" });
    expect(toggle).not.toBeChecked();
    expect(toggle).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Time" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Clock" } });
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    expect(lastPostAttributes().active).toBe(false);
  });

  it("rejects an empty privacy score with the in-page message", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add tool" });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Time" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Clock" } });
    fireEvent.change(screen.getByRole("spinbutton", { name: "Privacy level score" }), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    expect(await screen.findByText("Privacy level must be between 0 and 100")).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalledWith("/tools", expect.anything());
  });
});
