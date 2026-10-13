import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import { validatePriceMultipliers, rowsFromPriceMultipliers, PRICE_TIERS_BY_VENDOR } from "./LLMPriceMultipliersField";
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
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <LLMForm />
      </MemoryRouter>
    </ThemeProvider>,
  );

const storedLLM = (attrs) => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Claude",
        vendor: "anthropic",
        privacy_score: 10,
        api_endpoint: "https://api.anthropic.com/v1/",
        api_key: "$SECRET/anthropic",
        filters: [],
        plugins: [],
        allowed_models: [],
        default_model: "",
        short_description: "",
        long_description: "",
        logo_url: "",
        namespace: "",
        vendor_workspace_id: "",
        price_multipliers: null,
        ...attrs,
      },
    },
  },
});

const pickVendor = async (name) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: /Vendor/ }));
  fireEvent.click(await screen.findByRole("option", { name }));
};

const sentAttributes = (method) => {
  const call = apiClient[method].mock.calls.find((c) => String(c[0]).startsWith("/llms"));
  return call[1].data.attributes;
};

describe("validatePriceMultipliers", () => {
  it("builds the map and ignores fully empty rows", () => {
    const { map, rowErrors } = validatePriceMultipliers([
      { tier: "batch", value: "0.4" },
      { tier: "", value: "" },
      { tier: "flex", value: "0" },
    ]);
    expect(rowErrors).toEqual({});
    expect(map).toEqual({ batch: 0.4, flex: 0 });
  });

  it("returns null for no overrides", () => {
    expect(validatePriceMultipliers([]).map).toBeNull();
    expect(validatePriceMultipliers([{ tier: "", value: "" }]).map).toBeNull();
  });

  it("names bad tiers, duplicates and out-of-range or missing values", () => {
    const { rowErrors } = validatePriceMultipliers([
      { tier: "Batch", value: "0.5" },
      { tier: "batch", value: "11" },
      { tier: "batch", value: "0.5" },
      { tier: "priority", value: "" },
      { tier: "", value: "2" },
      { tier: "flex", value: "abc" },
    ]);
    expect(rowErrors[0].tier).toMatch(/lowercase/);
    expect(rowErrors[1].value).toMatch(/between 0 and 10/);
    expect(rowErrors[2].tier).toMatch(/more than once/);
    expect(rowErrors[3].value).toMatch(/Enter a multiplier/);
    expect(rowErrors[4].tier).toMatch(/Enter a tier/);
    expect(rowErrors[5].value).toMatch(/between 0 and 10/);
  });

  it("rejects default and auto for every vendor, since they are recorded as no tier", () => {
    const rows = [
      { tier: "default", value: "0.5" },
      { tier: "auto", value: "0.5" },
      { tier: "flex", value: "0.5" },
    ];
    for (const vendor of ["openai", "anthropic", undefined]) {
      const { rowErrors, map } = validatePriceMultipliers(rows, vendor);
      expect(rowErrors[0].tier).toMatch(/billed at list price and recorded as no tier/);
      expect(rowErrors[1].tier).toMatch(/billed at list price and recorded as no tier/);
      expect(rowErrors[2]).toBeUndefined();
      expect(map).toEqual({ flex: 0.5 });
    }
  });

  it("turns a stored map into rows", () => {
    expect(rowsFromPriceMultipliers({ batch: 0.4 })).toEqual([{ tier: "batch", value: "0.4" }]);
    expect(rowsFromPriceMultipliers(null)).toEqual([]);
  });
});

describe("LLMForm price multipliers", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/llms") return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    apiClient.put.mockResolvedValue({ data: {} });
  });

  it("is offered for Anthropic and OpenAI, not for other vendors", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add LLM provider" });
    expect(screen.queryByRole("button", { name: /Add price override/ })).not.toBeInTheDocument();
    await pickVendor(/Ollama/);
    expect(screen.queryByRole("button", { name: /Add price override/ })).not.toBeInTheDocument();
    await pickVendor(/OpenAI/);
    expect(screen.getByRole("button", { name: /Add price override/ })).toBeInTheDocument();
    await pickVendor(/Anthropic/);
    expect(screen.getByRole("button", { name: /Add price override/ })).toBeInTheDocument();
    expect(screen.getByText(/not retroactive/)).toBeInTheDocument();
  });

  it("suggests flex, priority and scale for OpenAI and blocks a default tier", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "GPT" } });
    await pickVendor(/OpenAI/);
    expect(PRICE_TIERS_BY_VENDOR.openai).toEqual(["flex", "priority", "scale"]);

    fireEvent.click(screen.getByRole("button", { name: /Add price override/ }));
    expect(screen.getByLabelText("Tier 1")).toHaveValue("flex");
    fireEvent.change(screen.getByLabelText("Tier 1"), { target: { value: "default" } });
    fireEvent.change(screen.getByLabelText("Multiplier 1"), { target: { value: "0.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    expect(await screen.findByText(/billed at list price and recorded as no tier/)).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("sends null when no override is set", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms", expect.anything()));
    expect(sentAttributes("post").price_multipliers).toBeNull();
  });

  it("adds a batch row first and sends the override map on create", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.click(screen.getByRole("button", { name: /Add price override/ }));
    expect(screen.getByLabelText("Tier 1")).toHaveValue("batch");
    fireEvent.change(screen.getByLabelText("Multiplier 1"), { target: { value: "0.4" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms", expect.anything()));
    expect(sentAttributes("post").price_multipliers).toEqual({ batch: 0.4 });
  });

  it("blocks an out-of-range multiplier before saving", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.click(screen.getByRole("button", { name: /Add price override/ }));
    fireEvent.change(screen.getByLabelText("Multiplier 1"), { target: { value: "12" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(screen.getByTestId("llm-form-errors")).toHaveTextContent("price multipliers"));
    expect(screen.getByText(/between 0 and 10/)).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("loads stored overrides on edit and sends null once they are all removed", async () => {
    mockParams = { id: "5" };
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms/5") return Promise.resolve(storedLLM({ price_multipliers: { batch: 0.4, priority: 1.2 } }));
      if (url === "/filters") return Promise.resolve({ data: [] });
      return Promise.resolve({ data: { data: [] } });
    });
    renderForm();
    await waitFor(() => expect(screen.getByLabelText("Tier 2")).toHaveValue("priority"));
    expect(screen.getByLabelText("Multiplier 1")).toHaveValue(0.4);

    fireEvent.click(screen.getByRole("button", { name: "Remove price override 2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update LLM provider" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledTimes(1));
    expect(sentAttributes("patch").price_multipliers).toEqual({ batch: 0.4 });

    fireEvent.click(screen.getByRole("button", { name: "Remove price override 1" }));
    fireEvent.click(screen.getByRole("button", { name: "Update LLM provider" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledTimes(2));
    expect(apiClient.patch.mock.calls[1][1].data.attributes.price_multipliers).toBeNull();
  });

  it("shows the server's validation error", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 400, data: { errors: [{ title: "Bad Request", detail: "price_multipliers: value for \"batch\" out of range" }] } },
    });
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.click(screen.getByRole("button", { name: /Add price override/ }));
    fireEvent.change(screen.getByLabelText("Multiplier 1"), { target: { value: "0.4" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    expect(await screen.findAllByText(/price_multipliers: value for "batch" out of range/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});
