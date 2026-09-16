import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../utils/testTheme";
import TykConnectionForm, { formToInput, emptyForm } from "./TykConnectionForm";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({
    can: () => true,
    canAny: () => true,
    canAll: () => true,
    isFullAdmin: true,
    hasAdminAccess: true,
    rbacEnabled: false,
  }),
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const renderForm = (path) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/admin/tyk-connections/new" element={<TykConnectionForm />} />
          <Route path="/admin/tyk-connections/edit/:id" element={<TykConnectionForm />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

const enabled = { data: { available: true, enabled: true } };

const existing = {
  id: 7,
  name: "Prod Dashboard",
  dashboard_url: "https://dash.example.com",
  gateway_base_url: "https://gw.example.com",
  template_id: "gov-defaults",
  declared_mode: "full",
  effective_mode: "full",
  status: "active",
  capabilities: { mcp_write: { state: "denied", detail: "403" }, template_read: { state: "ok", detail: "Governance" } },
  data_planes: [{ group_id: "eu", tags: ["edge-eu"], node_count: 2, healthy: true }],
  gateway_tags: ["edge-eu"],
  known_gateway_tags: [{ tag: "edge-eu" }],
  gateway_base_urls: { "edge-eu": "https://eu.example.com" },
  key_defaults: { alias_prefix: "studio:", expires_in_seconds: 0 },
  accept_handoffs: false,
  lock_version: 3,
  has_token: true,
};

describe("formToInput", () => {
  it("sends the template id trimmed, omits empty tokens and clears the score on edit", () => {
    const input = formToInput({ ...emptyForm, name: "n", dashboard_url: "https://d", template_id: "  gov  " }, true);
    expect(input.template_id).toBe("gov");
    expect(input).not.toHaveProperty("dashboard_access_token");
    expect(input.clear_default_privacy_score).toBe(true);
    expect(input.key_defaults).toEqual({ alias_prefix: "studio:", expires_in_seconds: 0 });
  });
});

describe("TykConnectionForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve(enabled);
      if (path === "/tyk-connections/7") return Promise.resolve({ data: existing });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockResolvedValue({ data: { id: 5 } });
    apiClient.patch.mockResolvedValue({ data: { ...existing, lock_version: 4 } });
  });

  it("creates a connection with the template id and navigates back with a snackbar", async () => {
    renderForm("/admin/tyk-connections/new");
    expect(await screen.findByText("Connect a Tyk Dashboard")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("name-input"), { target: { value: "New" } });
    fireEvent.change(screen.getByTestId("dashboard-url-input"), { target: { value: "https://dash.example.com" } });
    fireEvent.change(screen.getByTestId("token-input"), { target: { value: "secret-token-1234" } });
    fireEvent.change(screen.getByTestId("template-id-input"), { target: { value: "gov-defaults" } });

    fireEvent.click(screen.getByTestId("save-button"));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith(
        "/tyk-connections",
        expect.objectContaining({
          name: "New",
          dashboard_url: "https://dash.example.com",
          dashboard_access_token: "secret-token-1234",
          template_id: "gov-defaults",
          declared_mode: "catalogue",
        }),
      ),
    );
    expect(mockNavigate).toHaveBeenCalledWith("/admin/tyk-connections", {
      state: { snackbar: { message: "Connection created; activate it to start syncing", severity: "success" } },
    });
  });

  it("tests unsaved settings and shows the probe panel", async () => {
    apiClient.post.mockImplementation((path) => {
      if (path === "/tyk-connections/probe") {
        return Promise.resolve({
          data: {
            reachable: true,
            effective_mode: "catalogue",
            org_id: "org-1",
            capabilities: { mcp_read: { state: "ok" }, template_read: { state: "no", detail: "template not found" } },
            warnings: ["API template gone was not found"],
            data_planes: [],
          },
        });
      }
      return Promise.reject(new Error("unexpected " + path));
    });
    renderForm("/admin/tyk-connections/new");
    await screen.findByText("Connect a Tyk Dashboard");
    fireEvent.change(screen.getByTestId("dashboard-url-input"), { target: { value: "https://dash.example.com" } });
    fireEvent.change(screen.getByTestId("token-input"), { target: { value: "secret-token-1234" } });

    fireEvent.click(screen.getByTestId("probe-button"));
    expect(await screen.findByTestId("probe-panel")).toHaveTextContent("Effective mode: Catalogue");
    expect(apiClient.post).toHaveBeenCalledWith(
      "/tyk-connections/probe",
      expect.objectContaining({ dashboard_url: "https://dash.example.com", dashboard_access_token: "secret-token-1234" }),
    );
    expect(screen.getByTestId("capability-template_read")).toHaveTextContent("API template: no");
    expect(screen.getByText("API template gone was not found")).toBeInTheDocument();
  });

  it("loads an existing connection, shows its capabilities and patches with the lock version", async () => {
    renderForm("/admin/tyk-connections/edit/7");
    expect(await screen.findByText("Edit connection")).toBeInTheDocument();
    expect(screen.getByTestId("name-input")).toHaveValue("Prod Dashboard");
    expect(screen.getByTestId("template-id-input")).toHaveValue("gov-defaults");
    expect(screen.getByTestId("capabilities-section")).toBeInTheDocument();
    expect(screen.getByTestId("capability-mcp_write")).toHaveTextContent("Create MCP proxies: denied");
    expect(screen.getByText(/eu: 2 node\(s\), tags edge-eu/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Dashboard access token \(leave empty to keep\)/)).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("template-id-input"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("save-button"));
    await waitFor(() =>
      expect(apiClient.patch).toHaveBeenCalledWith(
        "/tyk-connections/7",
        expect.objectContaining({ name: "Prod Dashboard", template_id: "", accept_handoffs: false, lock_version: 3 }),
      ),
    );
    const body = apiClient.patch.mock.calls[0][1];
    expect(body).not.toHaveProperty("dashboard_access_token");
    expect(body.gateway_base_urls).toEqual({ "edge-eu": "https://eu.example.com" });
    expect(mockNavigate).toHaveBeenCalledWith("/admin/tyk-connections", {
      state: { snackbar: { message: "Connection saved", severity: "success" } },
    });
  });

  it("probes the saved connection from the capabilities section", async () => {
    apiClient.post.mockImplementation((path) => {
      if (path === "/tyk-connections/7/probe") {
        return Promise.resolve({ data: { reachable: true, effective_mode: "full", capabilities: { mcp_write: { state: "ok" } }, warnings: [], data_planes: [] } });
      }
      return Promise.reject(new Error("unexpected " + path));
    });
    renderForm("/admin/tyk-connections/edit/7");
    fireEvent.click(await screen.findByTestId("probe-now"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/7/probe", {}));
    expect(await screen.findByTestId("probe-panel")).toHaveTextContent("Effective mode: Full");
    expect(await screen.findByText("Probe complete")).toBeInTheDocument();
  });

  it("refuses to test unsaved settings on edit without a token", async () => {
    renderForm("/admin/tyk-connections/edit/7");
    await screen.findByText("Edit connection");
    fireEvent.click(screen.getByTestId("probe-button"));
    expect(await screen.findByTestId("form-error")).toHaveTextContent(/Enter the access token/);
    expect(apiClient.post).not.toHaveBeenCalled();
  });
});
