import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../../../utils/testTheme";
import apiClient from "../../../../utils/apiClient";
import QuickConnectForm from "./QuickConnectForm";

jest.mock("../../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

let mockCanExecute = true;
jest.mock("../../../../context/PermissionsContext", () => ({
  usePermissions: () => ({ can: (perm) => (perm === "tyk-connections:execute" ? mockCanExecute : true) }),
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const renderForm = (props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <QuickConnectForm onCreated={jest.fn()} onCancel={jest.fn()} {...props} />
      </MemoryRouter>
    </ThemeProvider>,
  );

const fill = () => {
  fireEvent.change(screen.getByTestId("quick-connect-name"), { target: { value: "Prod" } });
  fireEvent.change(screen.getByTestId("quick-connect-url"), { target: { value: "https://dash.example.com" } });
  fireEvent.change(screen.getByTestId("quick-connect-token"), { target: { value: "user-key" } });
};

describe("QuickConnectForm", () => {
  beforeEach(() => {
    mockCanExecute = true;
    jest.clearAllMocks();
  });

  it("keeps the token a password field and gates the buttons on the required fields", () => {
    renderForm();
    expect(screen.getByTestId("quick-connect-token")).toHaveAttribute("type", "password");
    expect(screen.getByTestId("quick-connect-probe")).toBeDisabled();
    expect(screen.getByTestId("quick-connect-save")).toBeDisabled();
    fill();
    expect(screen.getByTestId("quick-connect-probe")).toBeEnabled();
    expect(screen.getByTestId("quick-connect-save")).toBeEnabled();
  });

  it("probes with the settings-page payload and shows the result", async () => {
    apiClient.post.mockResolvedValue({
      data: { reachable: true, effective_mode: "catalogue", org_id: "org-1", capabilities: { apis_read: { state: "ok" } } },
    });
    renderForm();
    fill();
    fireEvent.change(screen.getByTestId("quick-connect-org"), { target: { value: "org-1" } });
    fireEvent.click(screen.getByTestId("quick-connect-probe"));
    await screen.findByTestId("probe-panel");
    expect(apiClient.post).toHaveBeenCalledTimes(1);
    const [url, body] = apiClient.post.mock.calls[0];
    expect(url).toBe("/tyk-connections/probe");
    expect(body).toMatchObject({
      name: "Prod",
      dashboard_url: "https://dash.example.com",
      dashboard_access_token: "user-key",
      org_id: "org-1",
      declared_mode: "catalogue",
      allow_internal_host: false,
      sync_interval_seconds: 300,
    });
    expect(screen.getByTestId("capability-apis_read")).toBeInTheDocument();
  });

  it("saves and hands the new connection back", async () => {
    const onCreated = jest.fn();
    apiClient.post.mockResolvedValue({ data: { id: 7, name: "Prod", status: "pending" } });
    renderForm({ onCreated });
    fill();
    fireEvent.click(screen.getByTestId("quick-connect-internal"));
    fireEvent.click(screen.getByTestId("quick-connect-save"));
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith({ id: 7, name: "Prod", status: "pending" }));
    const [url, body] = apiClient.post.mock.calls[0];
    expect(url).toBe("/tyk-connections");
    expect(body.allow_internal_host).toBe(true);
  });

  it("surfaces a save error and hides the internal-host switch without execute", async () => {
    mockCanExecute = false;
    apiClient.post.mockRejectedValue({ response: { data: { errors: [{ detail: "dashboard_url: internal host" }] } } });
    renderForm();
    expect(screen.queryByTestId("quick-connect-internal")).not.toBeInTheDocument();
    fill();
    fireEvent.click(screen.getByTestId("quick-connect-save"));
    await waitFor(() => expect(screen.getByTestId("quick-connect-error")).toHaveTextContent("internal host"));
  });

  it("links to the full settings form", () => {
    renderForm();
    fireEvent.click(screen.getByTestId("quick-connect-settings"));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/tyk-connections/new");
  });
});
