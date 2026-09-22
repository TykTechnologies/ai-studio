import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import LLMDetails from "./LLMDetails";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="chart" /> }));
jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: false }),
}));
jest.mock("../metadata/GovernedMetadataSummary", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../common/UsedBySection", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../common/ExportProxyLogsModal", () => ({
  __esModule: true,
  default: () => null,
}));

const llm = (active) => ({
  data: {
    data: {
      id: "7",
      attributes: {
        name: "OpenAI Prod",
        vendor: "openai",
        active,
        privacy_score: 2,
        api_endpoint: "https://api.openai.com",
        credential_status: "ok",
      },
    },
  },
});

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/llms/7"]}>
        <PermissionsProvider identity={identity(permissions)}>
          <Routes>
            <Route path="/admin/llms/:id" element={<LLMDetails />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const mockGets = (active = true) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/llms/7") return Promise.resolve(llm(active));
    if (url === "/analytics/proxy-logs-for-llm") {
      return Promise.resolve({ data: { data: [], meta: { total_count: 0, total_pages: 0 } } });
    }
    if (url === "/analytics/usage") return Promise.resolve({ data: { labels: [], datasets: [] } });
    if (url === "/analytics/budget-usage") return Promise.resolve({ data: [] });
    if (url === "/analytics/total-cost-per-vendor-and-model") return Promise.resolve({ data: [] });
    return Promise.resolve({ data: { data: [] } });
  });
};

describe("LLMDetails edit/activate affordances", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    mockGets(true);
    apiClient.post.mockResolvedValue({ data: {} });
  });

  it("read-only: neither Edit LLM nor an activate/deactivate button", async () => {
    renderWith(["llms:read"]);
    await screen.findByText("LLM provider details");
    expect(screen.queryByRole("button", { name: /Edit LLM/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Deactivate LLM provider/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Activate LLM provider/ })).not.toBeInTheDocument();
  });

  it("write-only: Edit LLM but no activate/deactivate", async () => {
    renderWith(["llms:write"]);
    await screen.findByText("LLM provider details");
    expect(screen.getByRole("button", { name: /Edit LLM/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Deactivate LLM provider/ })).not.toBeInTheDocument();
  });

  it("publish-only: a Deactivate button that calls the deactivate endpoint and refreshes, no Edit", async () => {
    renderWith(["llms:publish"]);
    await screen.findByText("LLM provider details");
    expect(screen.queryByRole("button", { name: /Edit LLM/ })).not.toBeInTheDocument();

    const fetchesBefore = apiClient.get.mock.calls.filter(([url]) => url === "/llms/7").length;
    fireEvent.click(screen.getByRole("button", { name: "Deactivate LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms/7/deactivate"));
    await waitFor(() =>
      expect(apiClient.get.mock.calls.filter(([url]) => url === "/llms/7").length).toBeGreaterThan(fetchesBefore),
    );
  });

  it("publish-only: an inactive provider offers Activate", async () => {
    mockGets(false);
    renderWith(["llms:publish"]);
    await screen.findByText("LLM provider details");
    fireEvent.click(screen.getByRole("button", { name: "Activate LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms/7/activate"));
  });
});
