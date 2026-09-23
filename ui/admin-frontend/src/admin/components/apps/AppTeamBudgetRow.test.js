import React from "react";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AppTeamBudgetRow from "./AppTeamBudgetRow";
import { teamBudgetsService } from "../../services/teamBudgetsService";

jest.mock("../../services/teamBudgetsService", () => {
  const actual = jest.requireActual("../../services/teamBudgetsService");
  return { ...actual, teamBudgetsService: { getReport: jest.fn() } };
});

const Label = ({ children }) => <span>{children}</span>;

const renderRow = (attributes) =>
  render(
    <MemoryRouter>
      <AppTeamBudgetRow app={{ id: "7", attributes }} FieldLabel={Label} FieldValue={Label} />
    </MemoryRouter>,
  );

describe("AppTeamBudgetRow", () => {
  beforeEach(() => {
    teamBudgetsService.getReport.mockResolvedValue({
      team_name: "Engineering", enabled: true, managed: true, enforcement: "hard_block",
    });
  });

  it("links the team and explains its pool", async () => {
    renderRow({ team_id: 3, monthly_budget: 5 });
    expect(await screen.findByText("Engineering")).toHaveAttribute("href", "/admin/groups/3");
    expect(screen.getByTestId("app-team-pool-note")).toHaveTextContent("blocking budget applies");
  });

  it("says nothing about a pool the team does not have", async () => {
    teamBudgetsService.getReport.mockResolvedValue({ team_name: "Ops", enabled: true, managed: false });
    renderRow({ team_id: 4, monthly_budget: null });
    await screen.findByText("Ops");
    expect(screen.queryByTestId("app-team-pool-note")).not.toBeInTheDocument();
  });

  it("handles an App with no team", () => {
    renderRow({ team_id: null });
    expect(screen.getByText("Not attributed to a team")).toBeInTheDocument();
    expect(teamBudgetsService.getReport).not.toHaveBeenCalled();
  });
});
