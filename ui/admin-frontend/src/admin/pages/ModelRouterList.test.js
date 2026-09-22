import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import ModelRouterList from "./ModelRouterList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const routers = [
  { id: "1", attributes: { name: "Primary router", slug: "primary", description: "", pools: [], active: true } },
];

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity(permissions)}>
          <ModelRouterList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  const row = (await screen.findByText(name)).closest("tr");
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("ModelRouterList permission gating (model-routers:publish vs model-routers:write)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockResolvedValue({
      data: { data: routers },
      headers: { "x-total-count": "1", "x-total-pages": "1" },
    });
    apiClient.patch.mockResolvedValue({ data: {} });
  });

  it("publish-only: no Add Router, actions column with just the toggle, bulk activate/deactivate", async () => {
    renderWith(["model-routers:publish"]);
    await screen.findByText("Primary router");

    expect(screen.queryByRole("button", { name: /Add Router/ })).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: /Actions/ })).toBeInTheDocument();

    const menu = await openRowMenu("Primary router");
    expect(within(menu).getByRole("menuitem", { name: "Deactivate Router" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Edit Router/ })).not.toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Delete Router/ })).not.toBeInTheDocument();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Deactivate Router" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/model-routers/1/toggle"));

    fireEvent.click(screen.getByRole("checkbox", { name: "Select Primary router" }));
    const toolbar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    expect(within(toolbar).getByRole("button", { name: "Activate" })).toBeInTheDocument();
    expect(within(toolbar).getByRole("button", { name: "Deactivate" })).toBeInTheDocument();
    expect(within(toolbar).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("write-only: Add Router and edit/delete, but no toggle", async () => {
    renderWith(["model-routers:write"]);
    await screen.findByText("Primary router");

    expect(screen.getByRole("button", { name: /Add Router/ })).toBeInTheDocument();
    const menu = await openRowMenu("Primary router");
    expect(within(menu).getByRole("menuitem", { name: "Edit Router" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete Router" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Deactivate Router/ })).not.toBeInTheDocument();

    fireEvent.keyDown(menu, { key: "Escape" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Select Primary router" }));
    const toolbar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    expect(within(toolbar).getByRole("button", { name: "Delete" })).toBeInTheDocument();
    expect(within(toolbar).queryByRole("button", { name: "Activate" })).not.toBeInTheDocument();
  });

  it("read-only: the empty state has no Add Router button", async () => {
    apiClient.get.mockResolvedValue({ data: { data: [] }, headers: { "x-total-count": "0", "x-total-pages": "0" } });
    renderWith(["model-routers:read"]);
    await screen.findByText("Create your first Model Router");
    expect(screen.queryByRole("button", { name: /Add Router/ })).not.toBeInTheDocument();
  });
});
