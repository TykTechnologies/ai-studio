import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import ComplianceOverview from "./ComplianceOverview";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

// The tabs and cards have their own tests; here they are stubs, and the
// Filter Events tab reports the linked event it was given.
jest.mock("../components/compliance/FilterEventsTab", () => ({ eventId, onCloseEvent }) => (
  <div data-testid="filter-events-tab">
    <span data-testid="linked-event-id">{eventId || "none"}</span>
    <button onClick={onCloseEvent}>close link</button>
  </div>
));
jest.mock("../components/compliance/SummaryCards", () => () => <div />);
jest.mock("../components/compliance/HighRiskAppsTable", () => () => <div />);
jest.mock("../components/compliance/PolicyViolationsTab", () => () => <div data-testid="policy-tab" />);
jest.mock("../components/compliance/BudgetComplianceTab", () => () => <div />);
jest.mock("../components/compliance/ErrorsTab", () => () => <div />);
jest.mock("../components/compliance/AppRiskModal", () => () => null);
jest.mock("../components/common/DateRangePicker", () => () => <div />);

const Location = () => {
  const location = useLocation();
  return <span data-testid="location">{location.pathname + location.search}</span>;
};

const renderAt = (url) =>
  render(
    <MemoryRouter initialEntries={[url]}>
      <Routes>
        <Route
          path="/admin/compliance"
          element={
            <>
              <ComplianceOverview />
              <Location />
            </>
          }
        />
      </Routes>
    </MemoryRouter>
  );

describe("ComplianceOverview links to one event (?event=)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) =>
      Promise.resolve({ data: url === "/compliance/available" ? { available: true } : {} })
    );
  });

  test("?event=<id> opens the Filter Events tab with that event", async () => {
    renderAt("/admin/compliance?event=4242");
    expect(await screen.findByTestId("filter-events-tab")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Filter Events" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByTestId("linked-event-id")).toHaveTextContent("4242");
  });

  test("closing the linked event drops ?event= and keeps the tab", async () => {
    renderAt("/admin/compliance?event=4242&tab=x");
    fireEvent.click(await screen.findByText("close link"));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/admin/compliance?tab=x"));
    expect(screen.getByTestId("linked-event-id")).toHaveTextContent("none");
    expect(screen.getByRole("tab", { name: "Filter Events" })).toHaveAttribute("aria-selected", "true");
  });

  test("a malformed id is ignored", async () => {
    renderAt("/admin/compliance?event=1%20OR%201");
    expect(await screen.findByTestId("policy-tab")).toBeInTheDocument();
    expect(screen.queryByTestId("filter-events-tab")).not.toBeInTheDocument();
  });

  test("without ?event= the page opens on its first tab", async () => {
    renderAt("/admin/compliance");
    expect(await screen.findByTestId("policy-tab")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Policy Violations" })).toHaveAttribute("aria-selected", "true");
  });
});
