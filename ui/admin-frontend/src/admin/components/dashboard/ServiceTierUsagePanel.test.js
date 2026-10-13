import React from "react";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import ServiceTierUsagePanel, { tierLabel } from "./ServiceTierUsagePanel";
import apiClient from "../../utils/apiClient";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const usage = {
  by_tier: [
    { service_tier: "", requests: 10, total_tokens: 12000, cost_by_currency: { USD: 0.12 } },
    { service_tier: "batch", requests: 2, total_tokens: 1800, cost_by_currency: { USD: 0.003 } },
    { service_tier: "priority", requests: 1, total_tokens: 500, cost_by_currency: { USD: 0.01, EUR: 0.02 } },
  ],
  by_model: [
    { service_tier: "", vendor: "openai", model: "gpt-6", currency: "USD", requests: 10, prompt_tokens: 9000, response_tokens: 3000, total_tokens: 12000, cost: 0.12 },
    { service_tier: "batch", vendor: "anthropic", model: "claude-luna", currency: "USD", requests: 2, prompt_tokens: 1500, response_tokens: 300, total_tokens: 1800, cost: 0.003 },
    { service_tier: "priority", vendor: "openai", model: "gpt-6", currency: "USD", requests: 1, prompt_tokens: 400, response_tokens: 100, total_tokens: 500, cost: 0.01 },
  ],
};

const modelRows = () => within(screen.getByTestId("service-tier-models")).getAllByRole("row").slice(1);

describe("ServiceTierUsagePanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({ data: usage });
  });

  it("keeps untiered traffic apart from an explicit standard tier and shows unknown tiers as-is", () => {
    expect(tierLabel("")).toBe("Untiered (list price)");
    expect(tierLabel("standard")).toBe("Standard");
    expect(tierLabel("batch")).toBe("Batch");
    expect(tierLabel("flex")).toBe("Flex");
    expect(tierLabel("on_demand-2")).toBe("on_demand-2");
  });

  it("breaks the dashboard's range down by service tier", async () => {
    render(<ServiceTierUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    await screen.findByText("Service Tiers");
    expect(apiClient.get).toHaveBeenCalledWith("/analytics/service-tier-usage", {
      params: { start_date: "2026-10-01", end_date: "2026-10-08" },
    });

    const tiers = within(screen.getByTestId("service-tier-summary")).getAllByRole("row").slice(1);
    expect(tiers).toHaveLength(3);
    expect(within(tiers[0]).getByText("Untiered (list price)")).toBeInTheDocument();
    expect(within(tiers[1]).getByText("Batch")).toBeInTheDocument();
    expect(within(tiers[1]).getByText("1,800")).toBeInTheDocument();
    expect(within(tiers[1]).getByText("$0.0030")).toBeInTheDocument();
    // Several currencies are listed side by side, never summed.
    expect(within(tiers[2]).getByText("$0.0100 + 0.0200 EUR")).toBeInTheDocument();

    expect(modelRows()).toHaveLength(3);
  });

  it("filters the model breakdown by tier", async () => {
    render(<ServiceTierUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    await screen.findByText("Service Tiers");

    fireEvent.mouseDown(screen.getByRole("combobox", { name: /Service tier/ }));
    fireEvent.click(await screen.findByRole("option", { name: "Batch" }));
    expect(modelRows()).toHaveLength(1);
    expect(within(modelRows()[0]).getByText("claude-luna")).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox", { name: /Service tier/ }));
    fireEvent.click(await screen.findByRole("option", { name: "Untiered (list price)" }));
    expect(modelRows()).toHaveLength(1);
    expect(within(modelRows()[0]).getByText("$0.1200")).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox", { name: /Service tier/ }));
    fireEvent.click(await screen.findByRole("option", { name: "All tiers" }));
    expect(modelRows()).toHaveLength(3);
  });

  it("stays hidden when all traffic is untiered", async () => {
    apiClient.get.mockResolvedValue({ data: { by_tier: [usage.by_tier[0]], by_model: [usage.by_model[0]] } });
    const { container } = render(<ServiceTierUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("says so when the report cannot be loaded", async () => {
    apiClient.get.mockRejectedValue(new Error("boom"));
    render(<ServiceTierUsagePanel startDate="2026-10-01" endDate="2026-10-08" />);
    expect(await screen.findByText("Failed to load service tier usage")).toBeInTheDocument();
  });
});
