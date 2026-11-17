import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import ModelRouterDetails from "./ModelRouterDetails";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), patch: jest.fn(), put: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("../common/UsedBySection", () => ({ __esModule: true, default: () => null }));

// Workspaces E4: the router's actions are checked in its Workspace; a router
// the caller cannot see is not found.

const routerIn = (workspaceId) => ({
  data: {
    data: {
      id: "3",
      attributes: { name: "Primary router", slug: "primary", description: "", active: true, pools: [], catalogues: [], workspace_id: workspaceId },
    },
  },
});

// Write and publish in W1 (id 2) only.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["model-routers:read", "model-routers:write", "model-routers:publish"] }],
  },
};

const renderDetails = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/model-routers/3"]}>
        <PermissionsProvider identity={scoped}>
          <Routes>
            <Route path="/admin/model-routers/:id" element={<ModelRouterDetails />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const mockGet = (router) =>
  apiClient.get.mockImplementation((url) =>
    url === "/model-routers/3" ? router() : Promise.resolve({ data: { data: [] }, headers: {} }),
  );

describe("ModelRouterDetails Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
  });

  it("a router in the caller's Workspace offers Edit and the toggle", async () => {
    mockGet(() => Promise.resolve(routerIn(2)));
    renderDetails();
    await screen.findAllByText("Primary router");
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deactivate" })).toBeInTheDocument();
  });

  it("a router in another Workspace offers neither", async () => {
    mockGet(() => Promise.resolve(routerIn(3)));
    renderDetails();
    await screen.findAllByText("Primary router");
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate" })).not.toBeInTheDocument();
    // Catalogs in Workspaces the caller cannot see are left out.
    expect(screen.getByText(/Not in any catalog you can see/)).toBeInTheDocument();
  });

  it("a 404 renders the not-found panel", async () => {
    mockGet(() => Promise.reject({ response: { status: 404, data: {} } }));
    renderDetails();
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });
});
