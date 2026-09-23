import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import TeamBudgetsSwitch from "./TeamBudgetsSwitch";
import { teamBudgetsService } from "../../../services/teamBudgetsService";
import { useEdition } from "../../../context/EditionContext";

jest.mock("../../../context/EditionContext", () => ({
  useEdition: jest.fn(),
}));

jest.mock("../../../services/teamBudgetsService", () => {
  const actual = jest.requireActual("../../../services/teamBudgetsService");
  return {
    ...actual,
    teamBudgetsService: { getSettings: jest.fn(), setEnabled: jest.fn() },
  };
});

describe("TeamBudgetsSwitch", () => {
  beforeEach(() => {
    useEdition.mockReturnValue({ isEnterprise: true });
    teamBudgetsService.getSettings.mockResolvedValue({ enabled: false });
    teamBudgetsService.setEnabled.mockResolvedValue({ enabled: true });
  });

  it("renders nothing in Community Edition", () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    const { container } = render(<TeamBudgetsSwitch />);
    expect(container).toBeEmptyDOMElement();
    expect(teamBudgetsService.getSettings).not.toHaveBeenCalled();
  });

  it("switches team budgets on", async () => {
    render(<TeamBudgetsSwitch />);
    const toggle = await screen.findByTestId("team-budgets-switch");
    expect(toggle).not.toBeChecked();
    expect(screen.getByText(/no team budget is allocated or enforced/)).toBeInTheDocument();

    fireEvent.click(toggle);
    await waitFor(() => expect(teamBudgetsService.setEnabled).toHaveBeenCalledWith(true));
    await waitFor(() => expect(screen.getByTestId("team-budgets-switch")).toBeChecked());
    expect(screen.getByText(/The Default team starts with an empty pool/)).toBeInTheDocument();
  });
});
