import React from "react";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AppTeamBudgetRow from "./AppTeamBudgetRow";
import { teamBudgetsService } from "../../services/teamBudgetsService";
import { PermissionsProvider } from "../../context/PermissionsContext";

jest.mock("../../services/teamBudgetsService", () => {
  const actual = jest.requireActual("../../services/teamBudgetsService");
  return { ...actual, teamBudgetsService: { getReport: jest.fn() } };
});

const Label = ({ children }) => <span>{children}</span>;

const identity = (permissions) => ({ id: "1", attributes: { has_admin_access: true, rbac_enabled: true, permissions } });

const renderRow = (attributes, permissions = ["groups:read"]) =>
  render(
    <MemoryRouter>
      <PermissionsProvider identity={identity(permissions)}>
        <AppTeamBudgetRow app={{ id: "7", attributes }} FieldLabel={Label} FieldValue={Label} />
      </PermissionsProvider>
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

  // midsommar-clean-6bg5.10: the report needs teams read globally. Without
  // it (a role held only within a Workspace) the call is not made (it
  // answered 403) and the team is named by id, without a link to a page the
  // caller cannot open.
  it("names the team by id and asks for no report without groups:read", () => {
    teamBudgetsService.getReport.mockClear();
    renderRow({ team_id: 3, monthly_budget: 5 }, []);
    expect(screen.getByTestId("app-team-plain")).toHaveTextContent("Team 3");
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(teamBudgetsService.getReport).not.toHaveBeenCalled();
  });

  it("handles an App with no team", () => {
    teamBudgetsService.getReport.mockClear();
    renderRow({ team_id: null });
    expect(screen.getByText("Not attributed to a team")).toBeInTheDocument();
    expect(teamBudgetsService.getReport).not.toHaveBeenCalled();
  });
});
