import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
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

const WORKSPACE_LABEL = "Anthropic workspace ID";

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

describe("LLMForm Anthropic workspace pin", () => {
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

  it("is shown only for the Anthropic vendor, with help text about client headers", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add LLM provider" });
    expect(screen.queryByLabelText(WORKSPACE_LABEL)).not.toBeInTheDocument();

    await pickVendor(/OpenAI/);
    expect(screen.queryByLabelText(WORKSPACE_LABEL)).not.toBeInTheDocument();

    await pickVendor(/Anthropic/);
    expect(screen.getByLabelText(WORKSPACE_LABEL)).toBeInTheDocument();
    expect(screen.getByText(/client-supplied workspace headers are ignored/)).toBeInTheDocument();
  });

  it("sends the pin on create", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.change(screen.getByLabelText(WORKSPACE_LABEL), { target: { value: "wrkspc_01AbC-9" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms", expect.anything()));
    expect(sentAttributes("post").vendor_workspace_id).toBe("wrkspc_01AbC-9");
  });

  it("rejects an invalid ID before saving", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.change(screen.getByLabelText(WORKSPACE_LABEL), { target: { value: "bad id!" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(screen.getByTestId("llm-form-errors")).toHaveTextContent("workspace ID"));
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("loads the stored pin on edit and clears it with an empty value", async () => {
    mockParams = { id: "5" };
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms/5") return Promise.resolve(storedLLM({ vendor_workspace_id: "wrkspc_01stored" }));
      if (url === "/filters") return Promise.resolve({ data: [] });
      return Promise.resolve({ data: { data: [] } });
    });
    renderForm();
    const field = await screen.findByLabelText(WORKSPACE_LABEL);
    await waitFor(() => expect(field).toHaveValue("wrkspc_01stored"));

    fireEvent.change(field, { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Update LLM provider" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/llms/5", expect.anything()));
    expect(sentAttributes("patch").vendor_workspace_id).toBe("");
  });

  it("shows the server's validation error on the field", async () => {
    apiClient.post.mockRejectedValue({
      response: {
        status: 400,
        data: { errors: [{ title: "Bad Request", detail: "vendor_workspace_id must be 1-128 letters, digits, '_' or '-'" }] },
      },
    });
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Claude" } });
    await pickVendor(/Anthropic/);
    fireEvent.change(screen.getByLabelText(WORKSPACE_LABEL), { target: { value: "wrkspc_ok" } });
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    expect(await screen.findAllByText(/vendor_workspace_id must be 1-128/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});
