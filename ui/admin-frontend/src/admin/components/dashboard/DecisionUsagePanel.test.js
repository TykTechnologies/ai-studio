import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import DecisionUsagePanel from "./DecisionUsagePanel";
import apiClient from "../../utils/apiClient";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const usage = {
  totals: {
    requests: 3,
    questions: 6,
    answered: 5,
    refusals: 1,
    input_tokens: 1300,
    cost_by_currency: { USD: 0.1126 },
  },
  by_day: { labels: [], requests: [], questions: [], refusals: [], cost: [] },
  by_model: [
    { vendor: "systemone", model: "jev-1.13.0", currency: "USD", requests: 2, questions: 5, refusals: 1, input_tokens: 300, cost: 0.0126, cost_per_thousand: 6.3 },
    { vendor: "openai", model: "gpt-6-luna", currency: "USD", requests: 1, questions: 1, refusals: 0, input_tokens: 1000, cost: 0.1, cost_per_thousand: 100 },
  ],
};

describe("DecisionUsagePanel", () => {
  beforeEach(() => {
    apiClient.get.mockResolvedValue({ data: usage });
  });

  it("summarises decision traffic for the dashboard's range", async () => {
    render(<DecisionUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    await screen.findByText("Decision Models");
    expect(apiClient.get).toHaveBeenCalledWith("/analytics/decisions", {
      params: { start_date: "2026-10-01", end_date: "2026-10-08" },
    });

    expect(screen.getByTestId("decision-usage-requests")).toHaveTextContent("3");
    expect(screen.getByTestId("decision-usage-questions")).toHaveTextContent("5 / 6");
    expect(screen.getByTestId("decision-usage-refusals")).toHaveTextContent("16.7%");
    expect(screen.getByTestId("decision-usage-cost")).toHaveTextContent("$0.1126");

    const rows = screen.getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText("jev-1.13.0")).toBeInTheDocument();
    expect(within(rows[0]).getByText("$6.3000")).toBeInTheDocument();
  });

  it("stays hidden without decision traffic", async () => {
    apiClient.get.mockResolvedValue({
      data: { ...usage, totals: { ...usage.totals, requests: 0 }, by_model: [] },
    });
    const { container } = render(<DecisionUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("says so when the report cannot be loaded", async () => {
    apiClient.get.mockRejectedValue(new Error("boom"));
    render(<DecisionUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    expect(await screen.findByText("Failed to load decision model usage")).toBeInTheDocument();
  });
});
