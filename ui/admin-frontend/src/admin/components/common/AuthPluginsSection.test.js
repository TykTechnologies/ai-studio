import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";

// Opens the attach menu and picks a plugin, waiting for the menu to close.
const attach = async (name) => {
  fireEvent.mouseDown(await screen.findByRole("combobox"));
  const listbox = await screen.findByRole("listbox");
  fireEvent.click(within(listbox).getByText(name));
  await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
};
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import AuthPluginsSection from "./AuthPluginsSection";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { P } from "../../rbac/permissions";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), put: jest.fn() },
}));

const plugin = (id, name, hookType, hookTypes) => ({
  id: String(id),
  attributes: { name, hook_type: hookType, hook_types: hookTypes, description: "" },
});

// The /plugins list holds an auth plugin, a hybrid one whose auth hook is not
// primary, and one without the auth hook.
const pluginList = {
  data: {
    data: [
      plugin(1, "Entra IdP", "auth", ["auth"]),
      plugin(2, "Hybrid", "post_auth", ["post_auth", "auth"]),
      plugin(3, "Logger", "post_auth", ["post_auth"]),
    ],
  },
};

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <PermissionsProvider identity={identity(permissions)}>
        <AuthPluginsSection
          endpointPath="datasources"
          endpointId="5"
          noun="data source"
          writePermission={P.DATASOURCES_WRITE}
        />
      </PermissionsProvider>
    </ThemeProvider>,
  );

const mockGets = (attached) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/plugins") return Promise.resolve(pluginList);
    if (url === "/datasources/5/auth-plugins") return Promise.resolve({ data: { data: attached } });
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
};

describe("AuthPluginsSection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
  });

  it("shows the attached plugins in order", async () => {
    mockGets([plugin(2, "Hybrid", "post_auth", ["post_auth", "auth"]), plugin(1, "Entra IdP", "auth", ["auth"])]);
    renderWith(["datasources:write"]);

    const section = await screen.findByTestId("auth-plugins-section");
    await waitFor(() => expect(within(section).getByText("1.")).toBeInTheDocument());
    const rows = within(section).getAllByText(/^(Hybrid|Entra IdP)$/);
    expect(rows.map((r) => r.textContent)).toEqual(["Hybrid", "Entra IdP"]);
  });

  it("offers only plugins with the auth hook, and saves the new order", async () => {
    mockGets([]);
    apiClient.put.mockResolvedValue({ data: { message: "ok" } });
    renderWith(["datasources:write"]);

    expect(await screen.findByText(/authenticates with app keys/)).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox"));
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByText("Entra IdP")).toBeInTheDocument();
    expect(within(listbox).getByText("Hybrid")).toBeInTheDocument();
    expect(within(listbox).queryByText("Logger")).not.toBeInTheDocument();
    fireEvent.keyDown(listbox, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());

    await attach("Entra IdP");
    await attach("Hybrid");

    fireEvent.click(screen.getByLabelText("move Hybrid up"));
    fireEvent.click(screen.getByRole("button", { name: "Save auth plugins" }));

    await waitFor(() =>
      expect(apiClient.put).toHaveBeenCalledWith("/datasources/5/auth-plugins", { plugin_ids: [2, 1] }),
    );
    expect(await screen.findByText(/Auth plugins saved/)).toBeInTheDocument();
  });

  it("shows the save error the API returns", async () => {
    mockGets([]);
    apiClient.put.mockRejectedValue({
      response: { data: { errors: [{ detail: "invalid auth plugin: plugin \"x\" does not provide the auth hook" }] } },
    });
    renderWith(["datasources:write"]);

    await attach("Entra IdP");
    fireEvent.click(screen.getByRole("button", { name: "Save auth plugins" }));

    expect(await screen.findByText(/does not provide the auth hook/)).toBeInTheDocument();
  });

  it("is read-only without the write permission", async () => {
    mockGets([plugin(1, "Entra IdP", "auth", ["auth"])]);
    renderWith(["datasources:read"]);

    expect(await screen.findByText("Entra IdP")).toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save auth plugins" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("detach Entra IdP")).not.toBeInTheDocument();
  });
});
