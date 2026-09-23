import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import UserForm from "./UserForm";
import apiClient from "../../utils/apiClient";
import { usePermissions } from "../../context/PermissionsContext";
import {
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../../../components/unsaved-changes";

jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: false }),
}));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
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

const pickerMock = require("../../../test-utils/component-mocks").relationshipPickerMock;

const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

// Ordered so the mock picker's "Add" (appends options[0]) picks a team the
// user is not in, and "Remove" (drops value[0]) drops the loaded one.
const groups = [
  { id: "2", attributes: { name: "Platform" } },
  { id: "1", attributes: { name: "Default" } },
  { id: "3", attributes: { name: "Data" } },
];

const userPayload = () => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Dev", email: "dev@tyk.io", is_admin: false, show_portal: true, show_chat: true,
        email_verified: true, notifications_enabled: false, access_to_sso_config: false, roles: [],
      },
    },
  },
});

const DirtyProbe = () => {
  const { isDirty } = useUnsavedChanges();
  return <span data-testid="registry-dirty">{String(isDirty)}</span>;
};

const renderForm = ({ withGuard = false } = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        {withGuard ? (
          <UnsavedChangesProvider>
            <UserForm />
            <DirtyProbe />
          </UnsavedChangesProvider>
        ) : (
          <UserForm />
        )}
      </MemoryRouter>
    </ThemeProvider>,
  );

const teamPicker = () => screen.getByTestId("relationship-picker");
const teamItems = () =>
  within(teamPicker()).queryAllByTestId("relationship-picker-item").map((el) => el.textContent);

const callOrder = (fn) => fn.mock.invocationCallOrder[0];

describe("UserForm teams commit on save", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "5" };
    usePermissions.mockReturnValue({ rbacEnabled: false, identity: { id: "99", isFullAdmin: true } });
    apiClient.get.mockImplementation((url) => {
      if (url === "/groups") return Promise.resolve({ data: { data: groups } });
      if (url === "/users/5") return Promise.resolve(userPayload());
      if (url === "/users/5/groups") return Promise.resolve({ data: { data: [groups[1]] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    apiClient.post.mockImplementation((url) => {
      if (url === "/users") return Promise.resolve({ data: { data: { id: "123" } } });
      if (url === "/groups") return Promise.resolve({ data: { data: { id: "4", attributes: { name: "Growth" } } } });
      return Promise.resolve({ data: { data: {} } });
    });
    apiClient.delete.mockResolvedValue({ data: {} });
  });

  const waitForLoaded = async () => {
    await screen.findByDisplayValue("Dev");
    await waitFor(() => expect(teamItems()).toEqual(["Default"]));
  };

  // "Add to Team +" used to POST on click while every other field waited for
  // Update (F-03). Teams are now a picker whose diff is applied on save.
  it("shows the user's teams in a compact picker and commits the diff after the user PATCH", async () => {
    renderForm();
    await waitForLoaded();
    expect(teamPicker()).toHaveAttribute("data-item-label", "team");
    expect(teamPicker()).toHaveAttribute("data-variant", "compact");
    expect(within(teamPicker()).getByTestId("relationship-picker-options-count")).toHaveTextContent("3");

    fireEvent.click(within(teamPicker()).getByTestId("relationship-picker-add"));
    fireEvent.click(within(teamPicker()).getByTestId("relationship-picker-remove"));
    expect(teamItems()).toEqual(["Platform"]);
    expect(apiClient.post).not.toHaveBeenCalled();
    expect(apiClient.delete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Update user" }));

    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/users", {
        state: { snackbar: { message: "User updated successfully", severity: "success" } },
      }),
    );
    expect(apiClient.patch).toHaveBeenCalledWith("/users/5", expect.anything());
    expect(apiClient.post).toHaveBeenCalledWith("/groups/2/users", { data: { id: "5", type: "users" } });
    expect(apiClient.delete).toHaveBeenCalledWith("/groups/1/users/5");
    expect(callOrder(apiClient.patch)).toBeLessThan(callOrder(apiClient.post));
    expect(callOrder(apiClient.patch)).toBeLessThan(callOrder(apiClient.delete));
  });

  it("refuses to save a user out of every team as a validation error, not a blocked delete", async () => {
    renderForm();
    await waitForLoaded();
    fireEvent.click(within(teamPicker()).getByTestId("relationship-picker-remove"));
    expect(teamItems()).toEqual([]);

    fireEvent.click(screen.getByRole("button", { name: "Update user" }));

    // The mock picker records its props rather than rendering helperText.
    await waitFor(() => expect(pickerMock.getLastProps().helperText).toBe("User must be in at least one team"));
    expect(pickerMock.getLastProps().error).toBe(true);
    expect(apiClient.patch).not.toHaveBeenCalled();
    expect(apiClient.delete).not.toHaveBeenCalled();
  });

  it("creates a team inline, adds it to the selection, and assigns it on save", async () => {
    renderForm();
    await waitForLoaded();

    fireEvent.click(screen.getByRole("button", { name: "Create a new team" }));
    fireEvent.change(screen.getByLabelText("New team name"), { target: { value: "Growth" } });
    fireEvent.click(screen.getByRole("button", { name: "Create team" }));

    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/groups", {
        data: { type: "Group", attributes: { name: "Growth" } },
      }),
    );
    await waitFor(() => expect(teamItems()).toEqual(["Default", "Growth"]));
    expect(apiClient.post).not.toHaveBeenCalledWith("/groups/4/users", expect.anything());
    expect(await screen.findByText(/Team "Growth" created\. It is assigned to this user when you save\./)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Update user" }));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/groups/4/users", { data: { id: "5", type: "users" } }),
    );
    expect(apiClient.delete).not.toHaveBeenCalled();
  });

  it("on create, adds the picked teams after the POST returns the new id", async () => {
    mockParams = {};
    renderForm();
    await waitFor(() => expect(within(teamPicker()).getByTestId("relationship-picker-options-count")).toHaveTextContent("3"));
    expect(pickerMock.getLastProps().helperText).toBe("New users also join the Default team automatically.");

    fireEvent.change(screen.getByRole("textbox", { name: /^name/i }), { target: { value: "New Person" } });
    fireEvent.change(screen.getByRole("textbox", { name: /email/i }), { target: { value: "new@tyk.io" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "Secret#1" } });
    fireEvent.click(within(teamPicker()).getByTestId("relationship-picker-add"));
    expect(teamItems()).toEqual(["Platform"]);

    fireEvent.click(screen.getByRole("button", { name: "Add user" }));

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/users", expect.anything()));
    expect(apiClient.post).toHaveBeenCalledWith("/users", expect.anything());
    expect(apiClient.post).toHaveBeenCalledWith("/groups/2/users", { data: { id: "123", type: "users" } });
  });

  it("has a Cancel that returns to the list when clean, and registers team changes with the guard", async () => {
    renderForm({ withGuard: true });
    await waitForLoaded();
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));

    const cancel = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancel);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/users");

    fireEvent.click(within(teamPicker()).getByTestId("relationship-picker-add"));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));

    mockNavigate.mockClear();
    fireEvent.click(cancel);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
