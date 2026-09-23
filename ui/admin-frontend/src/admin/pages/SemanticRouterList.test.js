import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import SemanticRouterList from "./SemanticRouterList";
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
  {
    id: "1",
    attributes: {
      name: "Smart router",
      slug: "smart",
      active: true,
      models: ["smart/auto", "smart/complex"],
      settings: { mode: "shadow" },
      routes: [{ name: "simple" }, { name: "complex" }],
    },
  },
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
          <SemanticRouterList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  await screen.findByText(name);
  const row = screen.getByRole("row", { name: new RegExp(name) });
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("SemanticRouterList", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockResolvedValue({
      data: { data: routers, meta: { total_count: 1, total_pages: 1, page_size: 10, page_number: 1 } },
      headers: {},
    });
    apiClient.patch.mockResolvedValue({ data: {} });
  });

  it("lists the model string, route count and shadow mode", async () => {
    renderWith(["semantic-routers:read"]);
    await screen.findByText("Smart router");
    const row = screen.getByRole("row", { name: /Smart router/ });
    expect(row).toHaveTextContent("smart/auto");
    expect(row).toHaveTextContent("+1");
    expect(row).toHaveTextContent("2 route(s)");
    expect(within(row).getByText("Shadow")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/semantic-routers", { params: { page: 1, page_size: 10 } });
    expect(screen.queryByRole("button", { name: /Add Router/ })).not.toBeInTheDocument();
  });

  it("publish-only: the toggle sends the new active state; no edit or delete", async () => {
    renderWith(["semantic-routers:publish"]);
    const menu = await openRowMenu("Smart router");
    expect(within(menu).queryByRole("menuitem", { name: /Edit Router/ })).not.toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Delete Router/ })).not.toBeInTheDocument();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Deactivate Router" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/semantic-routers/1/toggle", { active: false }));
  });

  it("write + delete: Add Router, edit and delete, bulk delete only", async () => {
    renderWith(["semantic-routers:write", "semantic-routers:delete"]);
    expect(await screen.findByRole("button", { name: /Add Router/ })).toBeInTheDocument();
    const menu = await openRowMenu("Smart router");
    expect(within(menu).getByRole("menuitem", { name: "Edit Router" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete Router" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Deactivate Router/ })).not.toBeInTheDocument();

    fireEvent.keyDown(menu, { key: "Escape" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Select Smart router" }));
    const toolbar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    expect(within(toolbar).getByRole("button", { name: "Delete" })).toBeInTheDocument();
    expect(within(toolbar).queryByRole("button", { name: "Activate" })).not.toBeInTheDocument();
  });
});
