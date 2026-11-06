import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import TeamAppQuotaPanel from "./TeamAppQuotaPanel";
import { appQuotaService } from "../../../services/appQuotaService";

jest.mock("../../../services/appQuotaService", () => ({
  appQuotaService: {
    getTeamQuota: jest.fn(),
    setTeamQuota: jest.fn(),
    removeTeamQuota: jest.fn(),
  },
}));

jest.mock("../../common/CollapsibleSection", () => ({
  __esModule: true,
  default: ({ title, children }) => (
    <section>
      <h2>{title}</h2>
      {children}
    </section>
  ),
}));

const inherited = { group_id: 3, set: false, max_apps_per_member: null, unlimited: false, default_max: 1, default_unlimited: false };
const five = { ...inherited, set: true, max_apps_per_member: 5 };

describe("TeamAppQuotaPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    appQuotaService.getTeamQuota.mockResolvedValue(inherited);
    appQuotaService.setTeamQuota.mockResolvedValue(five);
    appQuotaService.removeTeamQuota.mockResolvedValue();
  });

  it("shows the inherited default when the team sets none", async () => {
    render(<TeamAppQuotaPanel teamId={3} />);
    expect(await screen.findByTestId("team-app-quota-current")).toHaveTextContent("members get the default of 1 app per member");
    expect(screen.queryByTestId("team-app-quota-clear")).not.toBeInTheDocument();
  });

  it("saves a number and clears back to the default", async () => {
    render(<TeamAppQuotaPanel teamId={3} />);
    fireEvent.change(await screen.findByTestId("team-app-quota-input"), { target: { value: "5" } });
    fireEvent.click(screen.getByTestId("team-app-quota-save"));

    await waitFor(() => expect(appQuotaService.setTeamQuota).toHaveBeenCalledWith(3, { max_apps_per_member: 5 }));
    expect(await screen.findByText(/create 5 apps per member through the portal/)).toBeInTheDocument();

    appQuotaService.getTeamQuota.mockResolvedValue(inherited);
    fireEvent.click(screen.getByTestId("team-app-quota-clear"));
    await waitFor(() => expect(appQuotaService.removeTeamQuota).toHaveBeenCalledWith(3));
    expect(await screen.findByText(/members get the default/)).toBeInTheDocument();
  });

  it("saves unlimited", async () => {
    appQuotaService.setTeamQuota.mockResolvedValue({ ...inherited, set: true, unlimited: true });
    render(<TeamAppQuotaPanel teamId={3} />);
    fireEvent.click(await screen.findByTestId("team-app-quota-unlimited"));
    fireEvent.click(screen.getByTestId("team-app-quota-save"));
    await waitFor(() => expect(appQuotaService.setTeamQuota).toHaveBeenCalledWith(3, { unlimited: true }));
    expect(await screen.findByText(/create unlimited through the portal/)).toBeInTheDocument();
  });

  it("refuses a bad number without calling the API, and shows API errors", async () => {
    render(<TeamAppQuotaPanel teamId={3} />);
    fireEvent.change(await screen.findByTestId("team-app-quota-input"), { target: { value: "-2" } });
    fireEvent.click(screen.getByTestId("team-app-quota-save"));
    expect(await screen.findByText(/Enter a whole number/)).toBeInTheDocument();
    expect(appQuotaService.setTeamQuota).not.toHaveBeenCalled();

    appQuotaService.setTeamQuota.mockRejectedValue({ response: { data: { errors: [{ detail: "team not found" }] } } });
    fireEvent.change(screen.getByTestId("team-app-quota-input"), { target: { value: "2" } });
    fireEvent.click(screen.getByTestId("team-app-quota-save"));
    expect(await screen.findByText("team not found")).toBeInTheDocument();
  });
});
