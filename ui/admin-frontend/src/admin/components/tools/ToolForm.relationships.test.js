import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import ToolForm from "./ToolForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
} from "../../services/governedMetadataService";
import {
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../../../components/unsaved-changes";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
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

// Lists are ordered so the mock picker's "Add" (appends options[0]) picks
// something not already selected, and "Remove" (drops value[0]) drops the
// loaded member.
const allTools = [
  { id: "9", attributes: { name: "Time", description: "Clock" } },
  { id: "8", attributes: { name: "Geo", description: "Coordinates" } },
];
const allFilters = [
  { id: "4", attributes: { name: "Redact", response_filter: true } },
  { id: "3", attributes: { name: "PII", response_filter: false } },
];

const toolPayload = () => ({
  data: {
    data: {
      id: "7",
      governed_metadata: {},
      attributes: {
        name: "weather", description: "Forecasts", tool_type: "REST", oas_spec: "",
        privacy_score: 5, operations: [], file_stores: [], filters: [], namespace: "",
      },
    },
  },
});

const DirtyProbe = () => {
  const { isDirty } = useUnsavedChanges();
  return <span data-testid="registry-dirty">{String(isDirty)}</span>;
};

const renderForm = ({ withGuard = false } = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        {withGuard ? (
          <UnsavedChangesProvider>
            <ToolForm />
            <DirtyProbe />
          </UnsavedChangesProvider>
        ) : (
          <ToolForm />
        )}
      </MemoryRouter>
    </ThemeProvider>,
  );

const picker = (itemLabel) =>
  screen.getAllByTestId("relationship-picker").find((el) => el.dataset.itemLabel === itemLabel);

const pickerItems = (itemLabel) =>
  within(picker(itemLabel)).queryAllByTestId("relationship-picker-item").map((el) => el.textContent);

const callOrder = (fn) => fn.mock.invocationCallOrder[0];

describe("ToolForm dependencies and filters commit on save", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "7" };
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: allFilters });
      if (url === "/tools") return Promise.resolve({ data: { data: allTools } });
      if (url === "/tools/7") return Promise.resolve(toolPayload());
      if (url === "/tools/7/operations") return Promise.resolve({ data: { data: { operations: [] } } });
      if (url === "/tools/7/filters") return Promise.resolve({ data: { data: [allFilters[1]] } });
      if (url === "/tools/7/dependencies") return Promise.resolve({ data: { data: [allTools[1]] } });
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

  const waitForLoaded = async () => {
    await screen.findByDisplayValue("weather");
    await waitFor(() => expect(pickerItems("dependency")).toEqual(["Geo"]));
    await waitFor(() => expect(pickerItems("filter")).toEqual(["PII"]));
  };

  it("calls the section Filters (not Middleware) with the plain helper sentence", async () => {
    renderForm();
    await waitForLoaded();
    expect(screen.getByRole("button", { name: "Filters" })).toBeInTheDocument();
    expect(screen.queryByText(/Middleware/)).not.toBeInTheDocument();
    expect(screen.getByText(/Filters govern this tool's traffic\./)).toBeInTheDocument();
  });

  // Dependencies and filters used to POST/DELETE the moment "+" or the bin
  // was clicked, while the rest of the form waited for Update (F-03). They
  // are now pickers whose diff is applied on save, after the tool PATCH.
  it("issues the add/remove calls only on save, after the tool PATCH", async () => {
    renderForm();
    await waitForLoaded();

    fireEvent.click(within(picker("dependency")).getByTestId("relationship-picker-add"));
    fireEvent.click(within(picker("dependency")).getByTestId("relationship-picker-remove"));
    fireEvent.click(within(picker("filter")).getByTestId("relationship-picker-add"));
    fireEvent.click(within(picker("filter")).getByTestId("relationship-picker-remove"));
    expect(pickerItems("dependency")).toEqual(["Time"]);
    expect(pickerItems("filter")).toEqual(["Redact"]);
    expect(apiClient.post).not.toHaveBeenCalled();
    expect(apiClient.delete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/tools", expect.anything()));
    expect(apiClient.patch).toHaveBeenCalledWith("/tools/7", expect.anything());
    expect(apiClient.post).toHaveBeenCalledWith("/tools/7/dependencies/9");
    expect(apiClient.delete).toHaveBeenCalledWith("/tools/7/dependencies/8");
    expect(apiClient.post).toHaveBeenCalledWith("/tools/7/filters/4");
    expect(apiClient.delete).toHaveBeenCalledWith("/tools/7/filters/3");
    expect(callOrder(apiClient.patch)).toBeLessThan(callOrder(apiClient.post));
    expect(callOrder(apiClient.patch)).toBeLessThan(callOrder(apiClient.delete));
  });

  it("reports a refused dependency (circular reference) after the tool itself is saved", async () => {
    apiClient.post.mockRejectedValue({
      response: { data: { errors: [{ detail: "adding this dependency would create a circular reference" }] } },
    });
    renderForm();
    await waitForLoaded();
    fireEvent.click(within(picker("dependency")).getByTestId("relationship-picker-add"));
    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));

    expect(await screen.findByText(/it would create a circular reference\. The tool itself was saved\./)).toBeInTheDocument();
    expect(apiClient.patch).toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("on create, adds the picked dependencies and filters after the POST returns the id", async () => {
    mockParams = {};
    renderForm();
    await waitFor(() => expect(within(picker("dependency")).getByTestId("relationship-picker-options-count")).toHaveTextContent("2"));
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "geo" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Maps" } });
    fireEvent.click(within(picker("dependency")).getByTestId("relationship-picker-add"));
    fireEvent.click(within(picker("filter")).getByTestId("relationship-picker-add"));

    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/tools", expect.anything()));
    expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything());
    expect(apiClient.post).toHaveBeenCalledWith("/tools/11/dependencies/9");
    expect(apiClient.post).toHaveBeenCalledWith("/tools/11/filters/4");
    expect(apiClient.delete).not.toHaveBeenCalled();
  });

  it("has a Cancel that returns to the list when clean, and registers picker changes with the guard", async () => {
    renderForm({ withGuard: true });
    await waitForLoaded();
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));

    const cancel = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancel);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/tools");

    fireEvent.click(within(picker("filter")).getByTestId("relationship-picker-add"));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));

    mockNavigate.mockClear();
    fireEvent.click(cancel);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
