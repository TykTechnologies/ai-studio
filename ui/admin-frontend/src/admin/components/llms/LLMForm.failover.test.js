import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import { INHERITED_ACCESS_NOTE, modelAllowed, validateFailover } from "./LLMFailoverSection";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
import { useEdition } from "../../context/EditionContext";

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
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({ id: "5" }),
}));

const llmList = [
  { id: "5", attributes: { name: "Primary", vendor: "openai", active: true, allowed_models: [], default_model: "gpt-4o", privacy_score: 50, namespace: "" } },
  { id: "7", attributes: { name: "Backup", vendor: "anthropic", active: true, allowed_models: ["^claude-.*$"], default_model: "claude-sonnet-4", privacy_score: 50, namespace: "" } },
];

const primary = (failover) => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Primary", vendor: "openai", api_endpoint: "https://api.openai.com/v1", api_key: "[redacted]",
        privacy_score: 50, active: true, filters: [], plugins: [], allowed_models: [], default_model: "gpt-4o",
        namespace: "", failover,
      },
    },
  },
});

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <LLMForm />
      </MemoryRouter>
    </ThemeProvider>
  );

const mockApi = (failover) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/filters") return Promise.resolve({ data: [] });
    if (url === "/llms/5") return Promise.resolve(primary(failover));
    if (url === "/llms") return Promise.resolve({ data: { data: llmList } });
    return Promise.resolve({ data: { data: [] } });
  });
};

describe("LLMForm failover", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: false });
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("shows the inherited-access note and the stored waterfall", async () => {
    mockApi({ targets: [{ llm_id: 7, model: "claude-sonnet-4" }] });
    renderForm();

    expect(await screen.findByText(INHERITED_ACCESS_NOTE)).toBeInTheDocument();
    const model = await screen.findByTestId("failover-model-0");
    expect(model).toHaveValue("claude-sonnet-4");
    expect(screen.getByTestId("failover-llm-select-0")).toHaveValue("7");
  });

  it("sends the waterfall in the PATCH payload", async () => {
    mockApi({ targets: [{ llm_id: 7, model: "claude-sonnet-4" }] });
    renderForm();
    await screen.findByTestId("failover-model-0");

    fireEvent.click(screen.getByRole("button", { name: /update llm|save|create/i }));

    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, payload] = apiClient.patch.mock.calls[0];
    expect(payload.data.attributes.failover).toEqual({
      targets: [{ llm_id: 7, model: "claude-sonnet-4" }],
      triggers: undefined,
    });
  });

  it("refuses a model the fallback does not allow before calling the API", async () => {
    mockApi({ targets: [{ llm_id: 7, model: "gpt-4o" }] });
    renderForm();
    await screen.findByTestId("failover-model-0");

    fireEvent.click(screen.getByRole("button", { name: /update llm|save|create/i }));

    expect(await screen.findByText(/is not in the allowed models of Backup/)).toBeInTheDocument();
    expect(apiClient.patch).not.toHaveBeenCalled();
  });

  it("sends null when the last fallback is removed so the PATCH clears it", async () => {
    mockApi({ targets: [{ llm_id: 7, model: "claude-sonnet-4" }] });
    renderForm();
    await screen.findByTestId("failover-model-0");

    fireEvent.click(screen.getByRole("button", { name: "remove fallback" }));
    fireEvent.click(screen.getByRole("button", { name: /update llm|save|create/i }));

    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, payload] = apiClient.patch.mock.calls[0];
    expect(payload.data.attributes.failover).toBeNull();
  });
});

describe("failover validation helpers", () => {
  const llmsById = {
    7: { id: 7, name: "Backup", allowed_models: ["^claude-.*$"], privacy_score: 50, namespace: "" },
    8: { id: 8, name: "Open", allowed_models: [], privacy_score: 10, namespace: "eu" },
  };
  const current = { privacy_score: 50, namespace: "" };

  it("matches unanchored like the server and treats an empty list as allow-all", () => {
    expect(modelAllowed(["gpt-4.*"], "legacy-gpt-4o")).toBe(true);
    expect(modelAllowed(["^gpt-4.*$"], "legacy-gpt-4o")).toBe(false);
    expect(modelAllowed([], "anything")).toBe(true);
    expect(modelAllowed(["("], "anything")).toBe(false);
  });

  it("names the failing rung", () => {
    expect(validateFailover({ targets: [{ llm_id: 7, model: "" }] }, llmsById, current)).toEqual({ 0: "A model is required" });
    expect(validateFailover({ targets: [{ llm_id: 7, model: "gpt-4o" }] }, llmsById, current)[0]).toMatch(/not in the allowed models/);
    expect(validateFailover({ targets: [{ llm_id: 8, model: "x" }] }, llmsById, current)[0]).toMatch(/lower privacy level/);
    expect(validateFailover({ targets: [{ llm_id: 8, model: "x" }] }, llmsById, { privacy_score: 0, namespace: "" })[0]).toMatch(/namespace/);
    expect(
      validateFailover({ targets: [{ llm_id: 7, model: "claude-a" }, { llm_id: 7, model: "claude-a" }] }, llmsById, current)
    ).toEqual({ 1: "Duplicate failover target" });
    expect(validateFailover({ targets: [{ llm_id: 7, model: "claude-a" }], triggers: { status_codes: [403] } }, llmsById, current)._).toMatch(/403/);
    expect(validateFailover({ targets: [{ llm_id: 7, model: "claude-a" }] }, llmsById, current)).toEqual({});
  });
});
