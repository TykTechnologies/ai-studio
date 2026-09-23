import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import UserForm from "./UserForm";
import apiClient from "../../utils/apiClient";
import { usePermissions } from "../../context/PermissionsContext";

jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: true }),
}));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: jest.fn(),
}));
jest.mock("../../services/rbacService", () => ({
  listRoles: jest.fn().mockResolvedValue([]),
  sortRoles: (r) => r,
}));
jest.mock("../../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});
jest.mock("../common/relationship-picker", () =>
  require("../../../test-utils/component-mocks").relationshipPickerMock,
);

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({ id: "5" }),
}));

const groups = [
  { id: "1", attributes: { name: "Default" } },
  { id: "2", attributes: { name: "Platform" } },
  { id: "3", attributes: { name: "Data" } },
];

const userPayload = (budgetTeamId) => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Dev", email: "dev@tyk.io", is_admin: false, show_portal: true, show_chat: true,
        email_verified: true, notifications_enabled: false, access_to_sso_config: false, roles: [],
        budget_team_id: budgetTeamId,
      },
    },
  },
});

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <UserForm />
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("UserForm budget team (Enterprise)", () => {
  let budgetTeam;
  beforeEach(() => {
    jest.clearAllMocks();
    budgetTeam = null;
    usePermissions.mockReturnValue({ rbacEnabled: false, identity: { id: "99", isFullAdmin: true } });
    apiClient.get.mockImplementation((url) => {
      if (url === "/groups") return Promise.resolve({ data: { data: groups } });
      if (url === "/users/5") return Promise.resolve(userPayload(budgetTeam));
      if (url === "/users/5/groups") return Promise.resolve({ data: { data: groups } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    apiClient.put.mockResolvedValue({});
  });

  it("sets the chosen budget team after saving the user", async () => {
    renderForm();
    await screen.findByDisplayValue("Dev");
    const select = await screen.findByRole("combobox", { name: "Budget team" });
    fireEvent.mouseDown(select);
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Data"));

    fireEvent.click(screen.getByRole("button", { name: "Update user" }));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/users/5/budget-team", { team_id: 3 }));
    expect(apiClient.patch.mock.invocationCallOrder[0]).toBeLessThan(apiClient.put.mock.invocationCallOrder[0]);
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
  });

  it("leaves an unchanged budget team alone", async () => {
    budgetTeam = 2;
    renderForm();
    await screen.findByDisplayValue("Dev");
    expect(await screen.findByRole("combobox", { name: "Budget team" })).toHaveTextContent("Platform");

    fireEvent.click(screen.getByRole("button", { name: "Update user" }));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    expect(apiClient.put).not.toHaveBeenCalled();
  });
});
