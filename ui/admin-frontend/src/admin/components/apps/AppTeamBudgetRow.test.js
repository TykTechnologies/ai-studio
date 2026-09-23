import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AppTeamBudgetRow from "./AppTeamBudgetRow";
import { teamBudgetsService } from "../../services/teamBudgetsService";

jest.mock("../../services/teamBudgetsService", () => {
  const actual = jest.requireActual("../../services/teamBudgetsService");
  return { ...actual, teamBudgetsService: { getReport: jest.fn(), adoptApp: jest.fn() } };
});

const Label = ({ children }) => <span>{children}</span>;

const renderRow = (attributes, props = {}) =>
  render(
    <MemoryRouter>
      <AppTeamBudgetRow app={{ id: "7", attributes }} FieldLabel={Label} FieldValue={Label} {...props} />
    </MemoryRouter>,
  );

describe("AppTeamBudgetRow", () => {
  beforeEach(() => {
    teamBudgetsService.getReport.mockResolvedValue({ team_name: "Engineering", enabled: true, managed: true });
    teamBudgetsService.adoptApp.mockResolvedValue({});
  });

  it("flags a team App with nothing allocated", async () => {
    renderRow({ team_id: 3, budget_source: "team", monthly_budget: 0 });
    expect(await screen.findByText("Engineering")).toHaveAttribute("href", "/admin/groups/3");
    expect(screen.getByText(/No allocation/)).toBeInTheDocument();
    expect(screen.queryByTestId("app-adopt-team-budget")).not.toBeInTheDocument();
  });

  it("moves a manually budgeted App into the pool", async () => {
    const onChanged = jest.fn();
    renderRow({ team_id: 3, budget_source: "", monthly_budget: 10 }, { onChanged });
    fireEvent.click(await screen.findByTestId("app-adopt-team-budget"));
    await waitFor(() => expect(teamBudgetsService.adoptApp).toHaveBeenCalledWith("7"));
    expect(onChanged).toHaveBeenCalled();
  });

  it("reports a refused move", async () => {
    teamBudgetsService.adoptApp.mockRejectedValue({ response: { data: { errors: [{ detail: "pool too small" }] } } });
    const onError = jest.fn();
    renderRow({ team_id: 3, budget_source: "", monthly_budget: 10 }, { onError });
    fireEvent.click(await screen.findByTestId("app-adopt-team-budget"));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("pool too small"));
  });

  it("offers no move when the team has no pool", async () => {
    teamBudgetsService.getReport.mockResolvedValue({ team_name: "Ops", enabled: true, managed: false });
    renderRow({ team_id: 4, budget_source: "", monthly_budget: null });
    await screen.findByText("Ops");
    expect(screen.queryByTestId("app-adopt-team-budget")).not.toBeInTheDocument();
  });
});
