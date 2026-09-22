import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
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
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: {} }),
}));
jest.mock("../../services/pluginService", () => ({
  __esModule: true,
  default: { listPlugins: jest.fn(), getAvailableHookTypes: jest.fn(), getHookTypeLabel: jest.fn() },
}));
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

// Ordered so the mock picker's "Add" (appends options[0]) picks the one
// filter that is not already assigned.
const filters = [
  { id: "4", attributes: { name: "Redact" } },
  { id: "3", attributes: { name: "PII" } },
];

const llmPayload = () => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Primary", vendor: "openai", api_endpoint: "https://api.openai.com/v1", api_key: "[redacted]",
        privacy_score: 50, active: true, filters: [{ id: 3, name: "PII" }], plugins: [], allowed_models: [],
        default_model: "gpt-4o", namespace: "", failover: null,
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
            <LLMForm />
            <DirtyProbe />
          </UnsavedChangesProvider>
        ) : (
          <LLMForm />
        )}
      </MemoryRouter>
    </ThemeProvider>,
  );

const filterPicker = () =>
  screen.getAllByTestId("relationship-picker").find((el) => el.dataset.itemLabel === "filter");

const filterItems = () =>
  within(filterPicker()).queryAllByTestId("relationship-picker-item").map((el) => el.textContent);

describe("LLMForm filters picker and commit semantics", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "5" };
    useEdition.mockReturnValue({ isEnterprise: true });
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: filters });
      if (url === "/llms/5") return Promise.resolve(llmPayload());
      if (url === "/llms") return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("shows the assigned filters in a compact picker and saves the new selection as ids", async () => {
    renderForm();
    await screen.findByDisplayValue("Primary");
    await waitFor(() => expect(filterItems()).toEqual(["PII"]));
    expect(filterPicker()).toHaveAttribute("data-variant", "compact");
    expect(within(filterPicker()).getByTestId("relationship-picker-options-count")).toHaveTextContent("2");

    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    fireEvent.click(within(filterPicker()).getByTestId("relationship-picker-add"));
    expect(filterItems()).toEqual(["PII", "Redact"]);

    fireEvent.click(screen.getByRole("button", { name: "Update LLM provider" }));

    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [url, body] = apiClient.patch.mock.calls[0];
    expect(url).toBe("/llms/5");
    expect(body.data.attributes.filters).toEqual([3, 4]);
    // The ordered plugin list is still saved through its own endpoint.
    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/llms/5/plugins", { plugin_ids: [] }));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/llms", expect.anything());
  });

  it("moves a filter up the chain and submits the new order", async () => {
    renderForm();
    await screen.findByDisplayValue("Primary");
    await waitFor(() => expect(filterItems()).toEqual(["PII"]));
    // A single filter has nothing to reorder.
    expect(screen.queryByTestId("llm-filter-order")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    fireEvent.click(within(filterPicker()).getByTestId("relationship-picker-add"));
    expect(filterItems()).toEqual(["PII", "Redact"]);
    expect(screen.getByText(/Filters run top to bottom; the first block wins/)).toBeInTheDocument();

    const rows = () => screen.getAllByTestId("llm-filter-order-item").map((el) => el.textContent);
    expect(rows()).toEqual(["1.PII", "2.Redact"]);
    expect(screen.getByRole("button", { name: "move filter 1 up" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "move filter 2 down" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "move filter 2 up" }));
    expect(rows()).toEqual(["1.Redact", "2.PII"]);
    expect(filterItems()).toEqual(["Redact", "PII"]);

    fireEvent.click(screen.getByRole("button", { name: "Update LLM provider" }));

    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, body] = apiClient.patch.mock.calls[0];
    expect(body.data.attributes.filters).toEqual([4, 3]);
  });

  it("has a Cancel that returns to the list when clean, and registers picker changes with the guard", async () => {
    renderForm({ withGuard: true });
    await screen.findByDisplayValue("Primary");
    await waitFor(() => expect(filterItems()).toEqual(["PII"]));
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    const cancel = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancel);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/llms");

    fireEvent.click(within(filterPicker()).getByTestId("relationship-picker-remove"));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));

    mockNavigate.mockClear();
    fireEvent.click(cancel);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
