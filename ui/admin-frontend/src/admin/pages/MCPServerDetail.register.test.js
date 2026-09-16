import React from "react";
import { screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { renderWithRoutesAndTheme } from "../../test-utils/render-with-theme";
import MCPServerDetail from "./MCPServerDetail";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

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

const definition = { openapi: "3.0.3", "x-tyk-api-gateway": { info: { id: "mcp-1" }, upstream: { url: "https://w.example.com" }, middleware: { global: { transformRequestHeaders: { add: [{ name: "Authorization", value: "***" }] } } } } };

const server = {
  id: 7,
  connection_id: 1,
  connection_name: "Prod",
  tyk_api_id: "mcp-1",
  name: "Weather",
  slug: "weather",
  kind: "remote",
  listen_path: "/weather/",
  endpoint_url: "https://gw.example.com/weather/mcp",
  auth_mode: "auth_token",
  auth_details: {},
  primitives: [],
  gateway_tags: { enabled: false, tags: [] },
  dashboard_state: "active",
  origin: "studio",
  privacy_score: 10,
  is_active: false,
  brokerable: false,
  lock_version: 1,
  definition: JSON.stringify(definition),
  definition_hash: "hash-1",
  tool_catalogue_ids: [2],
  tool_catalogues: [{ id: 2, name: "Ops tools" }],
  bundle: [],
};

const catalogues = {
  data: [
    { id: "2", type: "ToolCatalogue", attributes: { name: "Ops tools" } },
    { id: "3", type: "ToolCatalogue", attributes: { name: "Research" } },
  ],
};

const renderDetail = () =>
  renderWithRoutesAndTheme(null, {
    initialEntry: "/admin/mcp-servers/7",
    routes: [
      { path: "/admin/mcp-servers/:id", element: <MCPServerDetail /> },
      { path: "/admin/mcp-servers", element: <div data-testid="list-page" /> },
    ],
  });

const mockGets = (srv = server, mode = "full") => {
  apiClient.get.mockImplementation((path) => {
    if (path === "/mcp-servers/7") return Promise.resolve({ data: srv });
    if (path === "/tyk-connections/1/policies") return Promise.resolve({ data: [] });
    if (path === "/tyk-connections/1") return Promise.resolve({ data: { id: 1, name: "Prod", status: "active", effective_mode: mode } });
    if (path === "/tool-catalogues") return Promise.resolve({ data: catalogues });
    return Promise.reject(new Error("unexpected " + path));
  });
};

describe("MCPServerDetail registration controls", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("validates and pushes an edited definition with the loaded hash", async () => {
    mockGets();
    apiClient.post.mockImplementation((path) => {
      if (path.endsWith("?dry_run=1")) return Promise.resolve({ data: { definition: {}, warnings: ["Consumer authentication changes from auth_token to keyless; minted keys are re-evaluated."] } });
      return Promise.resolve({ data: { server: { ...server, definition_hash: "hash-2", upstream_url: "https://w2.example.com" }, warnings: [] } });
    });
    renderDetail();

    const editor = await screen.findByTestId("definition-editor");
    expect(editor.value).toContain('"***"');
    const edited = { ...definition, "x-tyk-api-gateway": { ...definition["x-tyk-api-gateway"], upstream: { url: "https://w2.example.com" } } };
    fireEvent.change(editor, { target: { value: JSON.stringify(edited) } });
    fireEvent.click(screen.getByTestId("validate-definition"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-servers/7/push?dry_run=1", { definition: edited, expected_hash: "hash-1", confirm_dashboard_origin: false }));
    expect(await screen.findByText(/minted keys are re-evaluated/)).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("push-definition"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-servers/7/push", { definition: edited, expected_hash: "hash-1", confirm_dashboard_origin: false }));
    expect(await screen.findByText("Pushed to the Dashboard")).toBeInTheDocument();
  });

  it("requires the confirmation for Dashboard-origin proxies and hides delete", async () => {
    mockGets({ ...server, origin: "dashboard" });
    renderDetail();
    await screen.findByTestId("definition-editor");
    expect(screen.getByTestId("push-definition")).toBeDisabled();
    fireEvent.click(screen.getByTestId("confirm-origin"));
    expect(screen.getByTestId("push-definition")).toBeEnabled();
    expect(screen.queryByTestId("delete-server")).not.toBeInTheDocument();
  });

  it("hides the editor, creator and delete outside full mode", async () => {
    mockGets(server, "broker");
    renderDetail();
    await screen.findByText(/As synced from the Dashboard/);
    expect(screen.queryByTestId("definition-editor")).not.toBeInTheDocument();
    expect(screen.queryByTestId("open-policy-creator")).not.toBeInTheDocument();
    expect(screen.queryByTestId("delete-server")).not.toBeInTheDocument();
  });

  it("saves tool catalogue membership with numeric ids and explains non-brokerable servers", async () => {
    mockGets({ ...server, auth_mode: "oauth21" });
    apiClient.put.mockResolvedValue({ data: { ...server, tool_catalogue_ids: [2, 3], tool_catalogues: [{ id: 2, name: "Ops tools" }, { id: 3, name: "Research" }] } });
    renderDetail();

    const picker = await screen.findByTestId("relationship-picker");
    expect(within(picker).getByLabelText("Remove Ops tools")).toBeInTheDocument();
    expect(screen.getByTestId("not-brokerable")).toHaveTextContent("an OAuth token from the advertised authorization server");

    const input = within(picker).getByRole("combobox", { name: "Add catalog" });
    fireEvent.mouseDown(input);
    fireEvent.click(await screen.findByRole("option", { name: "Research" }));
    expect(within(picker).getByLabelText("Remove Research")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("save-catalogues"));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/mcp-servers/7/catalogues", { tool_catalogue_ids: [2, 3] }));
    expect(await screen.findByText("Catalogs saved")).toBeInTheDocument();
  });

  it("creates and pins a consumption policy through the creator", async () => {
    mockGets();
    apiClient.post.mockResolvedValue({ data: { tyk_policy_id: "pol-new", name: "Gold" } });
    renderDetail();
    fireEvent.click(await screen.findByTestId("open-policy-creator"));
    const dialog = await screen.findByTestId("policy-creator");
    const kind = dialog.querySelector('[data-testid="policy-kind"]');
    fireEvent.mouseDown(kind.parentElement.querySelector('[role="combobox"]') || kind.parentElement);
    fireEvent.click(await screen.findByRole("option", { name: /Consumption/ }));
    fireEvent.change(screen.getByTestId("policy-name"), { target: { value: "Gold" } });
    fireEvent.change(screen.getByTestId("policy-rate"), { target: { value: "100" } });
    fireEvent.click(screen.getByTestId("policy-create"));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/1/policies", { kind: "consumption", name: "Gold", server_id: 7, pin: true, rate: 100, per: 60, quota_max: 0, quota_renewal_rate: 3600, key_expires_in: 0 })
    );
    expect(await screen.findByText(/Policy Gold created/)).toBeInTheDocument();
  });

  it("publishes through the switch and pins a bundle from the cached policies", async () => {
    const policies = [
      { tyk_policy_id: "acl-1", name: "Weather access", api_ids: ["mcp-1"], is_partitioned: true, partitions: { acl: true } },
      { tyk_policy_id: "gold", name: "Gold", api_ids: [], is_partitioned: true, partitions: { rate_limit: true } },
    ];
    apiClient.get.mockImplementation((path) => {
      if (path === "/mcp-servers/7") return Promise.resolve({ data: server });
      if (path === "/tyk-connections/1/policies") return Promise.resolve({ data: policies });
      if (path === "/tyk-connections/1") return Promise.resolve({ data: { id: 1, name: "Prod", status: "active", effective_mode: "full" } });
      if (path === "/tool-catalogues") return Promise.resolve({ data: catalogues });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockResolvedValue({ data: { ...server, is_active: true } });
    apiClient.put.mockResolvedValue({ data: { ...server, brokerable: true, bundle: [{ role: "access", policy: policies[0] }] } });
    renderDetail();

    fireEvent.click(await screen.findByRole("checkbox", { name: "Published" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-servers/7/activate", {}));
    expect(await screen.findByText("Published to the portal")).toBeInTheDocument();

    const access = screen.getByTestId("access-select");
    fireEvent.mouseDown(access.parentElement.querySelector('[role="combobox"]') || access.parentElement);
    fireEvent.click(await screen.findByRole("option", { name: /Weather access/ }));
    fireEvent.click(screen.getByTestId("save-bundle"));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/mcp-servers/7/bundle", { pins: [{ tyk_policy_id: "acl-1", role: "access" }] }));
    expect(await screen.findByText("Brokerable")).toBeInTheDocument();
  });

  it("deletes a Studio-registered proxy with force", async () => {
    mockGets();
    apiClient.delete.mockResolvedValue({});
    renderDetail();
    fireEvent.click(await screen.findByTestId("delete-server"));
    const dialog = await screen.findByTestId("delete-dialog");
    fireEvent.click(within(dialog).getByTestId("delete-force"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith("/mcp-servers/7?force=true"));
    expect(await screen.findByTestId("list-page")).toBeInTheDocument();
  });
});
