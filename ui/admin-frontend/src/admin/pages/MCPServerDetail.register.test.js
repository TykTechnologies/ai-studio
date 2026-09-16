import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
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

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
}));

const theme = createTheme();

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
  group_ids: [],
  bundle: [],
};

const renderDetail = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers/7"]}>
        <Routes>
          <Route path="/admin/mcp-servers/:id" element={<MCPServerDetail />} />
          <Route path="/admin/mcp-servers" element={<div data-testid="list-page" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const mockGets = (srv = server, mode = "full") => {
  apiClient.get.mockImplementation((path) => {
    if (path === "/mcp-servers/7") return Promise.resolve({ data: srv });
    if (path === "/tyk-connections/1/policies") return Promise.resolve({ data: [] });
    if (path === "/tyk-connections/1") return Promise.resolve({ data: { id: 1, name: "Prod", status: "active", effective_mode: mode } });
    if (path === "/groups") return Promise.resolve({ data: [] });
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

  it("requires the confirmation for Dashboard-origin proxies and hides the editor outside full mode", async () => {
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
    await screen.findByText(/Definition \(from the Dashboard/);
    expect(screen.queryByTestId("definition-editor")).not.toBeInTheDocument();
    expect(screen.queryByTestId("open-policy-creator")).not.toBeInTheDocument();
    expect(screen.queryByTestId("delete-server")).not.toBeInTheDocument();
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

  it("deletes a Studio-registered proxy with force", async () => {
    mockGets();
    apiClient.delete.mockResolvedValue({});
    renderDetail();
    fireEvent.click(await screen.findByTestId("delete-server"));
    fireEvent.click(await screen.findByTestId("delete-force"));
    fireEvent.click(screen.getByTestId("confirm-delete"));
    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith("/mcp-servers/7?force=true"));
    expect(await screen.findByTestId("list-page")).toBeInTheDocument();
  });
});
