import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import LLMModelDetails from "./LLMModelDetails";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="chart" /> }));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const llmResponse = {
  data: { data: { id: "7", attributes: { name: "OpenAI Prod", vendor: "openai" } } },
};

const usageResponse = {
  data: { labels: ["2026-09-10"], datasets: [{ data: [100] }, { data: [1.5] }, { data: [60] }, { data: [40] }, { data: [0] }, { data: [0] }] },
};

const appsResponse = {
  data: [
    {
      appId: 12,
      appName: "legacy-faq-bot",
      appDeleted: false,
      ownerUserId: 3,
      ownerEmail: "ops@example.com",
      requestCount: 24,
      totalTokens: 98000,
      totalCost: 0.08,
      firstUsed: "2026-08-14T09:00:00Z",
      lastUsed: "2026-08-31T09:00:00Z",
    },
    {
      appId: 87,
      appName: "support-summariser",
      appDeleted: false,
      ownerUserId: 5,
      ownerEmail: "j.doe@example.com",
      requestCount: 1180,
      totalTokens: 2000000,
      totalCost: 1.84,
      firstUsed: "2026-08-12T09:00:00Z",
      lastUsed: "2026-09-07T16:03:00Z",
    },
    {
      appId: 9999,
      appName: "Deleted app #9999",
      appDeleted: true,
      ownerUserId: 0,
      ownerEmail: "",
      requestCount: 2,
      totalTokens: 10,
      totalCost: 0,
      firstUsed: "2026-08-20T09:00:00Z",
      lastUsed: "2026-08-20T09:00:00Z",
    },
  ],
};

const renderPage = (search = "?model=gpt-3.5-turbo-0125") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[`/admin/llms/7/models${search}`]}>
        <Routes>
          <Route path="/admin/llms/:id/models" element={<LLMModelDetails />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("LLMModelDetails", () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    apiClient.get.mockReset();
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms/7") return Promise.resolve(llmResponse);
      if (url === "/analytics/usage") return Promise.resolve(usageResponse);
      if (url === "/analytics/apps-for-model") return Promise.resolve(appsResponse);
      return Promise.reject(new Error(`unexpected url ${url}`));
    });
  });

  it("requests usage and apps for the model scoped to the LLM entry", async () => {
    renderPage();
    await screen.findByTestId("apps-for-model-table");

    const usageCall = apiClient.get.mock.calls.find(([url]) => url === "/analytics/usage");
    expect(usageCall[1].params).toMatchObject({ llm_id: "7", model_name: "gpt-3.5-turbo-0125" });

    const appsCall = apiClient.get.mock.calls.find(([url]) => url === "/analytics/apps-for-model");
    expect(appsCall[1].params).toMatchObject({ llm_id: "7", model_name: "gpt-3.5-turbo-0125" });
  });

  it("lists apps newest-used first with a mailto link for the owner", async () => {
    renderPage();
    const table = await screen.findByTestId("apps-for-model-table");
    const rows = within(table).getAllByRole("row").slice(1);

    expect(rows[0]).toHaveTextContent("support-summariser");
    expect(rows[1]).toHaveTextContent("legacy-faq-bot");
    expect(rows[2]).toHaveTextContent("Deleted app #9999");

    const owner = within(rows[0]).getByRole("link", { name: "j.doe@example.com" });
    expect(owner).toHaveAttribute("href", "mailto:j.doe@example.com");
    expect(rows[2]).toHaveTextContent("Unknown");
  });

  it("re-sorts when a column header is clicked", async () => {
    renderPage();
    const table = await screen.findByTestId("apps-for-model-table");

    fireEvent.click(within(table).getByRole("button", { name: /^App$/ }));
    let rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("Deleted app #9999");
    expect(rows[1]).toHaveTextContent("legacy-faq-bot");
    expect(rows[2]).toHaveTextContent("support-summariser");

    fireEvent.click(within(table).getByRole("button", { name: /Requests/ }));
    rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("support-summariser");
    expect(rows[2]).toHaveTextContent("Deleted app #9999");
  });

  it("navigates to the app when its name is clicked, but not for deleted apps", async () => {
    renderPage();
    const table = await screen.findByTestId("apps-for-model-table");

    fireEvent.click(within(table).getByRole("button", { name: "support-summariser" }));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps/87");
    expect(within(table).queryByRole("button", { name: "Deleted app #9999" })).not.toBeInTheDocument();
  });

  it("shows an empty state when no app used the model", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms/7") return Promise.resolve(llmResponse);
      if (url === "/analytics/usage") return Promise.resolve(usageResponse);
      if (url === "/analytics/apps-for-model") return Promise.resolve({ data: [] });
      return Promise.reject(new Error(`unexpected url ${url}`));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/No app called gpt-3.5-turbo-0125/)).toBeInTheDocument();
    });
  });

  it("explains when the apps request fails", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms/7") return Promise.resolve(llmResponse);
      if (url === "/analytics/usage") return Promise.resolve(usageResponse);
      return Promise.reject(new Error("boom"));
    });
    renderPage();
    await screen.findByText(/Could not load the apps for this model/);
  });

  it("asks for a model when none is in the URL", async () => {
    renderPage("");
    await screen.findByText(/No model was given/);
    expect(apiClient.get).not.toHaveBeenCalledWith("/analytics/apps-for-model", expect.anything());
  });
});
