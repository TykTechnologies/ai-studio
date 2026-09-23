import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import TeamCostsTable from "./TeamCostsTable";
import { teamBudgetsService } from "../../services/teamBudgetsService";
import { useEdition } from "../../context/EditionContext";

jest.mock("../../context/EditionContext", () => ({ useEdition: jest.fn() }));
jest.mock("../../services/teamBudgetsService", () => {
  const actual = jest.requireActual("../../services/teamBudgetsService");
  return { ...actual, teamBudgetsService: { getTeamCosts: jest.fn() } };
});

const renderTable = () =>
  render(
    <MemoryRouter>
      <TeamCostsTable startDate="2026-09-01" endDate="2026-09-23" />
    </MemoryRouter>,
  );

describe("TeamCostsTable", () => {
  beforeEach(() => {
    useEdition.mockReturnValue({ isEnterprise: true });
    teamBudgetsService.getTeamCosts.mockResolvedValue({
      teams: [
        { team_id: 1, team_name: "Default", cost: 0, tokens: 0, requests: 0 },
        { team_id: 3, team_name: "Engineering", cost: 75, tokens: 12000, requests: 40 },
      ],
      unattributed: { cost: 25, tokens: 100, requests: 2 },
    });
  });

  it("lists teams by cost with their share, and spend no team owns", async () => {
    renderTable();
    await screen.findByText("Engineering");
    expect(teamBudgetsService.getTeamCosts).toHaveBeenCalledWith("2026-09-01", "2026-09-23");

    const rows = screen.getAllByRole("row").slice(1);
    expect(within(rows[0]).getByText("Engineering")).toHaveAttribute("href", "/admin/groups/3");
    expect(within(rows[0]).getByText("$75.00")).toBeInTheDocument();
    expect(within(rows[0]).getByText("75.0%")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Default")).toBeInTheDocument();
    expect(within(screen.getByTestId("team-costs-unattributed")).getByText("25.0%")).toBeInTheDocument();
  });

  it("marks a deleted team and does not link to it", async () => {
    teamBudgetsService.getTeamCosts.mockResolvedValue({
      teams: [
        { team_id: 4, team_name: "Research", cost: 10, tokens: 1, requests: 1, deleted: true },
        { team_id: 9, team_name: "Research", cost: 5, tokens: 1, requests: 1 },
      ],
      unattributed: { cost: 0, tokens: 0, requests: 0 },
    });
    renderTable();
    const deleted = await screen.findByTestId("team-costs-deleted-4");
    expect(deleted).toHaveTextContent("Research (deleted)");
    expect(within(deleted).queryByRole("link")).toBeNull();
    const live = screen.getAllByRole("link");
    expect(live).toHaveLength(1);
    expect(live[0]).toHaveAttribute("href", "/admin/groups/9");
  });

  it("renders nothing in Community Edition", async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    const { container } = renderTable();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(teamBudgetsService.getTeamCosts).not.toHaveBeenCalled();
  });
});
