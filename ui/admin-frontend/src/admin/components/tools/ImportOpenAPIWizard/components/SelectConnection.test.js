import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../../../utils/testTheme";
import SelectConnection from "./SelectConnection";

jest.mock("../../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

let mockCanWrite = true;
jest.mock("../../../../context/PermissionsContext", () => ({
  usePermissions: () => ({ can: (perm) => (perm === "tyk-connections:write" ? mockCanWrite : true) }),
}));

const connections = [
  { id: 1, name: "Prod", dashboard_url: "https://dash.example.com", status: "active", degraded: false, apis_read: "ok" },
  { id: 2, name: "Staging", dashboard_url: "https://stg.example.com", status: "pending", degraded: true, degraded_reason: "403 on keys", apis_read: "denied" },
];

const renderStep = (props = {}) => {
  const reload = props.reload || jest.fn().mockResolvedValue({ available: true, enabled: true });
  const onSelect = props.onSelect || jest.fn();
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <SelectConnection
          status={{ available: true, enabled: true }}
          connections={connections}
          loading={false}
          error=""
          selected={null}
          {...props}
          reload={reload}
          onSelect={onSelect}
        />
      </MemoryRouter>
    </ThemeProvider>,
  );
  return { reload, onSelect };
};

describe("SelectConnection", () => {
  beforeEach(() => {
    mockCanWrite = true;
  });

  it("loads on mount and lists connections with their state", async () => {
    const { reload, onSelect } = renderStep();
    await waitFor(() => expect(reload).toHaveBeenCalled());
    expect(screen.getByText("Prod")).toBeInTheDocument();
    expect(screen.getByTestId("connection-status-active")).toBeInTheDocument();
    expect(screen.getByTestId("connection-status-pending")).toHaveTextContent("degraded");
    expect(screen.getByText("Cannot read APIs")).toBeInTheDocument();
    fireEvent.click(screen.getByDisplayValue("1"));
    expect(onSelect).toHaveBeenCalledWith(connections[0]);
  });

  it("shows the enterprise prompt when the integration is unavailable", () => {
    renderStep({ status: { available: false, enabled: false }, connections: [] });
    expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
    expect(screen.queryByTestId("add-connection")).not.toBeInTheDocument();
  });

  it("shows the disabled notice when the integration is off", () => {
    renderStep({ status: { available: true, enabled: false, disabled_reason: "TYK_AI_SECRET_KEY must be configured" }, connections: [] });
    expect(screen.getByText(/TYK_AI_SECRET_KEY must be configured/)).toBeInTheDocument();
  });

  it("tells a user without write permission to ask an administrator", () => {
    mockCanWrite = false;
    renderStep({ connections: [] });
    expect(screen.getByTestId("no-connections")).toHaveTextContent("Ask an administrator");
    expect(screen.queryByTestId("add-connection")).not.toBeInTheDocument();
  });

  it("opens the inline form, then reloads and selects the new connection", async () => {
    const reload = jest.fn().mockResolvedValue({ available: true, enabled: true });
    const { onSelect } = renderStep({ connections: [], reload });
    expect(screen.getByTestId("no-connections")).toHaveTextContent("Add one below");
    fireEvent.click(screen.getByTestId("add-connection"));
    expect(screen.getByTestId("quick-connect")).toBeInTheDocument();

    const apiClient = require("../../../../utils/apiClient").default;
    apiClient.post.mockResolvedValue({
      data: { id: 9, name: "New", dashboard_url: "https://new.example.com", status: "pending", degraded: false, effective_mode: "catalogue", capabilities: {} },
    });
    fireEvent.change(screen.getByTestId("quick-connect-name"), { target: { value: "New" } });
    fireEvent.change(screen.getByTestId("quick-connect-url"), { target: { value: "https://new.example.com" } });
    fireEvent.change(screen.getByTestId("quick-connect-token"), { target: { value: "k" } });
    fireEvent.click(screen.getByTestId("quick-connect-save"));
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 9, name: "New", apis_read: "" })));
    expect(reload).toHaveBeenCalledTimes(2);
    expect(screen.queryByTestId("quick-connect")).not.toBeInTheDocument();
  });
});
