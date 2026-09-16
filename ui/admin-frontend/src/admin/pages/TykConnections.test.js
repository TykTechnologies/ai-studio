import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import TykConnections from "./TykConnections";
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
jest.mock("../components/rbac/Can", () => ({ children }) => (
  <>{typeof children === "function" ? children(true) : children}</>
));
jest.mock("../../components/common/Icon", () => (props) => <div data-testid="mock-icon">{props.name}</div>);

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <TykConnections />
      </MemoryRouter>
    </ThemeProvider>,
  );

const connections = [
  {
    id: 1,
    name: "Prod Dashboard",
    dashboard_url: "https://dash.example.com",
    declared_mode: "full",
    effective_mode: "broker",
    status: "active",
    degraded: false,
    capabilities: { mcp_read: { state: "ok" } },
    gateway_tags: ["edge-eu"],
    lock_version: 3,
    has_token: true,
  },
];

const enabled = { data: { available: true, enabled: true } };

describe("TykConnections", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve(enabled);
      if (path === "/tyk-connections") return Promise.resolve({ data: connections });
      if (path === "/tyk-connections/1/dependents") return Promise.resolve({ data: { data: { attributes: {} } } });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockResolvedValue({ data: {} });
    apiClient.delete.mockResolvedValue({ data: {} });
  });

  it("shows the enterprise upsell when the feature is unavailable", async () => {
    apiClient.get.mockResolvedValue({ data: { available: false, enabled: false } });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(screen.getByText(/Tyk Connections is an Enterprise Edition feature/)).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledTimes(1);
  });

  it("shows why the feature is disabled", async () => {
    apiClient.get.mockResolvedValue({ data: { available: true, enabled: false, disabled_reason: "TYK_AI_SECRET_KEY is not set" } });
    renderPage();
    expect(await screen.findByText(/TYK_AI_SECRET_KEY is not set/)).toBeInTheDocument();
  });

  it("lists connections in the shared table under the Tyk Connections heading", async () => {
    renderPage();
    expect(await screen.findByText("Prod Dashboard")).toBeInTheDocument();
    expect(screen.getByText("Tyk Connections")).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Tyk connections" })).toBeInTheDocument();
    expect(screen.getByTestId("mode-chip")).toHaveTextContent("Broker (declared Full)");
    expect(screen.getByTestId("connection-status-active")).toBeInTheDocument();
    expect(screen.getByText("1 tag(s)")).toBeInTheDocument();
  });

  it("shows the empty state with a Connect Dashboard action that opens the form page", async () => {
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve(enabled);
      if (path === "/tyk-connections") return Promise.resolve({ data: [] });
      return Promise.reject(new Error("unexpected " + path));
    });
    renderPage();
    expect(await screen.findByText("No Tyk Dashboard connected yet")).toBeInTheDocument();
    fireEvent.click(screen.getAllByTestId("add-connection")[0]);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/tyk-connections/new");
  });

  it("opens the edit page from the row menu", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Actions for Prod Dashboard"));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Edit connection" }));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/tyk-connections/edit/1");
  });

  it("activates a pending connection through the row menu after confirming", async () => {
    const pending = [{ ...connections[0], status: "pending", effective_mode: "catalogue", declared_mode: "catalogue" }];
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve(enabled);
      if (path === "/tyk-connections") return Promise.resolve({ data: pending });
      return Promise.reject(new Error("unexpected " + path));
    });
    renderPage();
    fireEvent.click(await screen.findByLabelText("Actions for Prod Dashboard"));
    expect(screen.queryByTestId("menu-sync")).not.toBeInTheDocument();
    fireEvent.click(await screen.findByTestId("menu-activate"));
    expect(await screen.findByText("Activate Prod Dashboard?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Activate" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/1/activate", {}));
    expect(await screen.findByText("Prod Dashboard activated")).toBeInTheDocument();
  });

  it("disables an active connection with a reason", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Actions for Prod Dashboard"));
    expect(screen.queryByTestId("menu-activate")).not.toBeInTheDocument();
    fireEvent.click(await screen.findByTestId("menu-disable"));
    fireEvent.change(await screen.findByTestId("action-reason"), { target: { value: "maintenance" } });
    fireEvent.click(screen.getByRole("button", { name: "Disable" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/1/disable", { reason: "maintenance" }));
  });

  it("deletes through the confirmation dialog", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Actions for Prod Dashboard"));
    fireEvent.click(await screen.findByTestId("menu-delete"));
    expect(await screen.findByText("Delete Prod Dashboard?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith("/tyk-connections/1"));
    expect(await screen.findByText("Prod Dashboard deleted")).toBeInTheDocument();
  });
});
