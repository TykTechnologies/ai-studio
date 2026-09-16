import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import MCPCredentials from "./MCPCredentials";
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
  StyledTableHeaderCell: ({ children, sx, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, sx, ...props }) => <td {...props}>{children}</td>,
  StyledTableRow: ({ children, hover, sx, ...props }) => <tr {...props}>{children}</tr>,
}));

const theme = createTheme();

const credential = {
  id: "cred-1",
  app_id: 4,
  app_name: "Weather app",
  connection_id: 1,
  connection_name: "Prod",
  status: "active",
  tyk_key_hash: "abcdef1234567890",
  key_hint: "7890",
  alias: "ai-studio:app:4",
  applied_policy_ids: ["pol-acl", "pol-gold"],
  external_policy_ids: ["pol-ops"],
  desired_policy_ids: ["pol-acl", "pol-gold", "pol-new"],
  drift: "pending_widen",
  drift_detail: "pol-new grants api-tickets",
  minted_at: "2026-09-16T10:00:00Z",
  expires_at: null,
};

const renderPage = (route = "/admin/mcp-credentials") =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={[route]}>
        <MCPCredentials />
      </MemoryRouter>
    </ThemeProvider>
  );

const mockGets = (rows = [credential], report = []) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
    if (url === "/tyk-connections") return Promise.resolve({ data: [{ id: 1, name: "Prod", status: "active" }] });
    if (url === "/mcp-credentials") return Promise.resolve({ data: { credentials: rows, total: rows.length, page: 1, page_size: 25 } });
    if (url === "/mcp-access-report") return Promise.resolve({ data: report });
    return Promise.reject(new Error("unexpected " + url));
  });
};

describe("MCPCredentials", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists minted keys with external policies and applies pending drift", async () => {
    mockGets();
    apiClient.post.mockResolvedValue({ data: { ...credential, drift: "none" } });
    renderPage();

    const row = await screen.findByTestId("credential-cred-1");
    expect(row).toHaveTextContent("Weather app");
    expect(row).toHaveTextContent("pol-ops");
    expect(screen.getByTestId("drift-pending_widen")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("apply-drift-cred-1"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-credentials/cred-1/apply-drift", {}));
    expect(await screen.findByTestId("page-notice")).toHaveTextContent("updated");
  });

  it("revokes with a reason after confirmation", async () => {
    mockGets();
    apiClient.post.mockResolvedValue({ data: { ...credential, status: "revoked" } });
    renderPage();

    fireEvent.click(await screen.findByTestId("revoke-cred-1"));
    fireEvent.change(screen.getByTestId("action-reason"), { target: { value: "offboarding" } });
    fireEvent.click(screen.getByTestId("confirm-action"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-credentials/cred-1/revoke", { reason: "offboarding" }));
  });

  it("mints a key from the admin page and reveals it once", async () => {
    mockGets([]);
    apiClient.post.mockResolvedValue({
      data: {
        key: "org1plaintextkey9",
        credential: { id: "cred-9", status: "active" },
        servers: [{ id: 7, name: "Weather", slug: "weather", endpoint_url: "https://gw.example.com/weather/mcp" }],
        skipped: [],
      },
    });
    renderPage();
    await screen.findByTestId("empty");

    fireEvent.click(screen.getByTestId("open-mint"));
    fireEvent.change(await screen.findByTestId("mint-app"), { target: { value: "4" } });
    // MUI Select: open and pick.
    fireEvent.mouseDown(screen.getByTestId("mint-connection").querySelector('[role="combobox"]') || screen.getByTestId("mint-connection").firstChild);
    fireEvent.click(await screen.findByRole("option", { name: "Prod" }));
    fireEvent.click(screen.getByTestId("mint-submit"));

    expect(await screen.findByTestId("revealed-key")).toHaveTextContent("org1plaintextkey9");
    expect(apiClient.post).toHaveBeenCalledWith("/mcp-credentials", { app_id: 4, connection_id: 1 });
    fireEvent.click(screen.getByTestId("reveal-close"));
    await waitFor(() => expect(screen.queryByText(/org1plaintextkey9/)).not.toBeInTheDocument());
  });

  it("shows the access report with grant kinds and keyless rows", async () => {
    mockGets([], [
      { grant_id: 1, app_id: 4, app_name: "Weather app", user_id: 2, user_email: "member@tyk.io", server_id: 7, server_name: "Weather", connection_id: 1, connection_name: "Prod", grant_kind: "key", granted_at: "2026-09-16T10:00:00Z", credential_id: "cred-1", credential_hash: "abcdef1234567890", credential_status: "active" },
      { grant_id: 2, app_id: 4, app_name: "Weather app", user_id: 2, user_email: "member@tyk.io", server_id: 8, server_name: "Tickets", connection_id: 1, connection_name: "Prod", grant_kind: "oauth", granted_at: "2026-09-16T10:00:00Z" },
    ]);
    renderPage("/admin/mcp-credentials?tab=report");

    const oauth = await screen.findByTestId("grant-2");
    expect(oauth).toHaveTextContent("OAuth");
    expect(oauth).toHaveTextContent("No key");
    expect(screen.getByTestId("grant-1")).toHaveTextContent("Tyk key");
    expect(apiClient.get).toHaveBeenCalledWith("/mcp-access-report", { params: {} });

    fireEvent.click(screen.getByTestId("report-include-revoked"));
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/mcp-access-report", { params: { include_revoked: "true" } }));
  });

  it("renders the upsell in Community Edition", async () => {
    apiClient.get.mockResolvedValue({ data: { available: false, enabled: false } });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/mcp-credentials", expect.anything());
  });
});
