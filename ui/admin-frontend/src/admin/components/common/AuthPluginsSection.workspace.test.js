import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import AuthPluginsSection from "./AuthPluginsSection";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { P } from "../../rbac/permissions";

// Workspaces .54: the auth plugins of a tool or data source are edited with
// write in the object's Workspace.

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), put: jest.fn() },
}));

const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["datasources:read", "datasources:write"] }],
  },
};

const renderFor = (workspaceId) =>
  render(
    <ThemeProvider theme={testTheme}>
      <PermissionsProvider identity={scoped}>
        <AuthPluginsSection
          endpointPath="datasources"
          endpointId="5"
          noun="data source"
          writePermission={P.DATASOURCES_WRITE}
          workspaceId={workspaceId}
        />
      </PermissionsProvider>
    </ThemeProvider>,
  );

describe("AuthPluginsSection in a Workspace", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockImplementation((url) =>
      url === "/plugins"
        ? Promise.resolve({ data: { data: [{ id: "1", attributes: { name: "Entra IdP", hook_type: "auth", hook_types: ["auth"] } }] } })
        : Promise.resolve({ data: { data: [] } }),
    );
  });

  it("is editable with write in the object's Workspace", async () => {
    renderFor(2);
    expect(await screen.findByRole("button", { name: "Save auth plugins" })).toBeInTheDocument();
  });

  it("is read-only for an object of another Workspace", async () => {
    renderFor(3);
    await screen.findByTestId("auth-plugins-section");
    expect(screen.queryByRole("button", { name: "Save auth plugins" })).not.toBeInTheDocument();
  });
});
