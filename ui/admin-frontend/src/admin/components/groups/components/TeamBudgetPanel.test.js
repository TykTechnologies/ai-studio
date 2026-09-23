import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import TeamBudgetPanel from "./TeamBudgetPanel";
import { teamBudgetsService } from "../../../services/teamBudgetsService";

jest.mock("../../../services/teamBudgetsService", () => {
  const actual = jest.requireActual("../../../services/teamBudgetsService");
  return {
    ...actual,
    teamBudgetsService: {
      getReport: jest.fn(),
      setBudget: jest.fn(),
      removeBudget: jest.fn(),
      resetBudget: jest.fn(),
    },
  };
});

jest.mock("../../common/CollapsibleSection", () => ({
  __esModule: true,
  default: ({ title, children }) => (
    <section>
      <h2>{title}</h2>
      {children}
    </section>
  ),
}));

const managedReport = {
  team_id: 3,
  team_name: "Engineering",
  enabled: true,
  managed: true,
  monthly_budget: 100,
  default_app_allocation: 25,
  enforcement: "hard_block",
  budget_start_date: null,
  period_start: "2026-09-01T00:00:00Z",
  period_end: "2026-10-01T00:00:00Z",
  spent: 120,
  chat_spent: 5,
  usage: 120,
  allocated: 125,
  unallocated: -25,
  over_budget: true,
  blocking: true,
  over_allocated: true,
  apps: [
    { app_id: 1, name: "search", owner_email: "a@x.io", allocation: 100, spent: 90 },
    { app_id: 2, name: "blocked", owner_email: "a@x.io", allocation: 0, spent: 0, blocked: true },
    { app_id: 3, name: "gone", owner_email: "a@x.io", allocation: null, spent: 25, deleted: true },
  ],
};

describe("TeamBudgetPanel", () => {
  beforeEach(() => {
    teamBudgetsService.getReport.mockResolvedValue(managedReport);
    teamBudgetsService.setBudget.mockResolvedValue({ ...managedReport, monthly_budget: 200 });
    teamBudgetsService.resetBudget.mockResolvedValue();
    teamBudgetsService.removeBudget.mockResolvedValue();
  });

  it("shows spend, allocation and both overshoot signals", async () => {
    render(<TeamBudgetPanel teamId={3} />);

    expect(await screen.findByTestId("team-budget-panel")).toBeInTheDocument();
    expect(screen.getByTestId("team-budget-amount")).toHaveTextContent("$100.00");
    expect(screen.getByTestId("team-budget-spent")).toHaveTextContent("$120.00 (120%)");
    expect(screen.getByTestId("team-budget-unallocated")).toHaveTextContent("$-25.00");
    expect(screen.getByTestId("team-over-budget")).toHaveTextContent(/being refused/);
    expect(screen.getByTestId("team-over-allocated")).toBeInTheDocument();
    expect(screen.getByText("Budget $0: refused")).toBeInTheDocument();
    expect(screen.getByText("Decommissioned")).toBeInTheDocument();
    expect(screen.getByText(/Includes \$5.00 of chat/)).toBeInTheDocument();
  });

  it("explains an unmanaged team and a switched-off feature", async () => {
    teamBudgetsService.getReport.mockResolvedValue({
      ...managedReport,
      enabled: false,
      managed: false,
      monthly_budget: null,
      over_budget: false,
      over_allocated: false,
      spent: 12,
      chat_spent: 0,
      apps: [],
    });
    render(<TeamBudgetPanel teamId={3} />);

    expect(await screen.findByText(/Team budgets are switched off/)).toBeInTheDocument();
    expect(screen.getByText(/has no budget. It spent \$12.00/)).toBeInTheDocument();
    expect(screen.getByTestId("team-budget-edit")).toHaveTextContent("Set budget");
  });

  it("saves the budget from the editor", async () => {
    render(<TeamBudgetPanel teamId={3} />);
    fireEvent.click(await screen.findByTestId("team-budget-edit"));

    fireEvent.change(screen.getByTestId("team-budget-input"), { target: { value: "200" } });
    fireEvent.change(screen.getByTestId("team-default-allocation-input"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("team-budget-save"));

    await waitFor(() =>
      expect(teamBudgetsService.setBudget).toHaveBeenCalledWith(3, {
        monthly_budget: 200,
        default_app_allocation: null,
        enforcement: "hard_block",
        budget_start_date: null,
      }),
    );
    await waitFor(() => expect(screen.getByTestId("team-budget-amount")).toHaveTextContent("$200.00"));
  });

  it("shows the server's reason when a save is refused", async () => {
    teamBudgetsService.setBudget.mockRejectedValue({
      response: { data: { errors: [{ detail: "invalid team budget input: monthly_budget must be zero or more" }] } },
    });
    render(<TeamBudgetPanel teamId={3} />);
    fireEvent.click(await screen.findByTestId("team-budget-edit"));
    fireEvent.click(screen.getByTestId("team-budget-save"));

    expect(await screen.findByText(/must be zero or more/)).toBeInTheDocument();
  });

  it("starts a new period", async () => {
    render(<TeamBudgetPanel teamId={3} />);
    fireEvent.click(await screen.findByTestId("team-budget-reset"));
    await waitFor(() => expect(teamBudgetsService.resetBudget).toHaveBeenCalledWith(3));
    await waitFor(() => expect(teamBudgetsService.getReport).toHaveBeenCalledTimes(2));
  });
});
