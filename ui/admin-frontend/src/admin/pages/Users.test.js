import React from "react";
import { render, screen, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import Users from "./Users";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
// Can's render-prop form (children as a function) is used to gate the table.
jest.mock("../components/rbac/Can", () => ({ children }) => (
  <>{typeof children === "function" ? children(true) : children}</>
));
// Toggled per describe: the list swaps its Account type column for Roles
// when fine-grained RBAC is on.
let mockRbacEnabled = false;
jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({ rbacEnabled: mockRbacEnabled, can: () => true, canAny: () => true, canAll: () => true }),
}));
jest.mock("../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: { feature_gateway: true, feature_portal: true, feature_chat: true } }),
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const users = [
  { id: "1", attributes: { name: "Root", email: "root@example.com", is_admin: true, role: "Super Admin", email_verified: true, auth_source: "admin" } },
  { id: "2", attributes: { name: "Dana", email: "dana@example.com", is_admin: false, role: "Developer", email_verified: true, auth_source: "local" } },
  { id: "3", attributes: { name: "Chatty", email: "chat@example.com", is_admin: false, email_verified: true, auth_source: "local" } },
];

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <Users />
      </MemoryRouter>
    </ThemeProvider>,
  );

// Without RBAC the list used to have an "Is Admin" Yes/No column, which was a
// third vocabulary for the account type next to the team member tables'
// Super Admin / Admin / Developer / Chat user badges (UX review M9, F-23).
describe("Users list account type column (RBAC off)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) => {
      if (url === "/users") return Promise.resolve({ data: { data: users }, headers: { "x-total-count": "3", "x-total-pages": "1" } });
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
  });

  it("shows an Account type column with the same badges as the team member tables", async () => {
    renderPage();
    const rootRow = (await screen.findByText("Root")).closest("tr");
    expect(screen.getByRole("columnheader", { name: /Account type/ })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: /Is Admin/ })).not.toBeInTheDocument();
    expect(within(rootRow).getByText("Super Admin")).toBeInTheDocument();
    expect(within(screen.getByText("Dana").closest("tr")).getByText("Developer")).toBeInTheDocument();
    // A user the API returned without a role falls back to the lowest tier.
    expect(within(screen.getByText("Chatty").closest("tr")).getByText("Chat user")).toBeInTheDocument();
  });
});

// With RBAC on, the Roles column shows effective roles: a role a user holds
// only through a team is badged with the team it comes from.
describe("Users list Roles column (RBAC on)", () => {
  const rbacUsers = [
    {
      id: "5",
      attributes: {
        name: "Teammate",
        email: "teammate@example.com",
        is_admin: false,
        email_verified: true,
        auth_source: "local",
        roles: [{ id: 3, name: "Editor", slug: "editor", is_system: true, via: "group", group_id: 4, group_name: "Platform" }],
      },
    },
    {
      id: "6",
      attributes: {
        name: "Direct",
        email: "direct@example.com",
        is_admin: false,
        email_verified: true,
        auth_source: "local",
        roles: [{ id: 2, name: "Viewer", slug: "viewer", is_system: true, via: "direct" }],
      },
    },
  ];

  beforeEach(() => {
    jest.clearAllMocks();
    mockRbacEnabled = true;
    apiClient.get.mockImplementation((url) => {
      if (url === "/users") return Promise.resolve({ data: { data: rbacUsers }, headers: { "x-total-count": "2", "x-total-pages": "1" } });
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
  });

  afterEach(() => {
    mockRbacEnabled = false;
  });

  it("badges a team-inherited role with the team it comes from", async () => {
    renderPage();
    const row = (await screen.findByText("Teammate")).closest("tr");
    expect(screen.getByRole("columnheader", { name: /Roles/ })).toBeInTheDocument();
    const badge = within(row).getByTestId("role-badge-editor");
    expect(badge).toHaveTextContent("Editor");
    expect(badge).toHaveTextContent("via team Platform");
    expect(badge).toHaveAttribute("data-via", "group");
    expect(badge.className).toMatch(/MuiChip-outlined/);

    const directRow = screen.getByText("Direct").closest("tr");
    const directBadge = within(directRow).getByTestId("role-badge-viewer");
    expect(directBadge).toHaveTextContent("Viewer");
    expect(directBadge).not.toHaveTextContent("via team");
    expect(directBadge.className).toMatch(/MuiChip-filled/);
  });
});
