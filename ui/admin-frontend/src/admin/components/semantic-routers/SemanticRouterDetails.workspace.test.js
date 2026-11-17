import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import SemanticRouterDetails from "./SemanticRouterDetails";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), patch: jest.fn(), post: jest.fn(), put: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("../common/UsedBySection", () => ({ __esModule: true, default: () => null }));

// Workspaces E4: the router's actions (edit, toggle, test) are checked in its
// Workspace; a router the caller cannot see is not found.

const routerIn = (workspaceId) => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Smart",
        slug: "smart",
        active: true,
        namespace: "",
        models: ["smart/auto"],
        catalogues: [],
        workspace_id: workspaceId,
        settings: { mode: "enforce", judge: { enabled: false, model_ref: {} }, affinity: { enabled: false }, default_route: "simple" },
        routes: [{ name: "simple", description: "", target: { type: "llm", llm_id: 1, model: "gpt-4o-mini" } }],
      },
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
    workspaces: [
      { id: 2, name: "W1", permissions: ["semantic-routers:read", "semantic-routers:write", "semantic-routers:publish"] },
    ],
  },
};

const renderDetails = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/semantic-routers/5"]}>
        <PermissionsProvider identity={scoped}>
          <Routes>
            <Route path="/admin/semantic-routers/:id" element={<SemanticRouterDetails />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const mockGet = (router) =>
  apiClient.get.mockImplementation((url) =>
    url === "/semantic-routers/5" ? router() : Promise.resolve({ data: { data: [] }, headers: {} }),
  );

describe("SemanticRouterDetails Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
  });

  it("a router in the caller's Workspace offers Edit, the toggle and Test", async () => {
    mockGet(() => Promise.resolve(routerIn(2)));
    renderDetails();
    await screen.findAllByText("Smart");
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deactivate" })).toBeInTheDocument();
    expect(screen.getByText("Classify a prompt with the saved configuration.")).toBeInTheDocument();
  });

  it("a router in another Workspace offers none of them", async () => {
    mockGet(() => Promise.resolve(routerIn(3)));
    renderDetails();
    await screen.findAllByText("Smart");
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate" })).not.toBeInTheDocument();
    expect(screen.queryByText("Classify a prompt with the saved configuration.")).not.toBeInTheDocument();
    expect(screen.getByText(/Not in any catalog you can see/)).toBeInTheDocument();
  });

  it("a 404 renders the not-found panel", async () => {
    mockGet(() => Promise.reject({ response: { status: 404, data: {} } }));
    renderDetails();
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });
});
