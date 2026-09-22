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
  default: { get: jest.fn(), patch: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../common/UsedBySection", () => ({
  __esModule: true,
  default: () => null,
}));

const router = {
  data: {
    data: {
      id: "3",
      attributes: { name: "Primary router", slug: "primary", description: "", active: true, pools: [] },
    },
  },
};

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/model-routers/3"]}>
        <PermissionsProvider identity={identity(permissions)}>
          <Routes>
            <Route path="/admin/model-routers/:id" element={<ModelRouterDetails />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("ModelRouterDetails edit/toggle affordances", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockResolvedValue(router);
  });

  it("publish-only: toggle shown, Edit hidden", async () => {
    renderWith(["model-routers:publish"]);
    expect(await screen.findByRole("button", { name: "Deactivate" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("write-only: Edit shown, toggle hidden", async () => {
    renderWith(["model-routers:write"]);
    expect(await screen.findByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate" })).not.toBeInTheDocument();
  });
});
