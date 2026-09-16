import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import MCPServerDetail from "./MCPServerDetail";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({ can: () => true, canAny: () => true, canAll: () => true, isFullAdmin: true, hasAdminAccess: true, rbacEnabled: false }),
}));

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
}));

const theme = createTheme();

const pending = {
  id: 9,
  connection_id: 1,
  connection_name: "Catalogue",
  tyk_api_id: "",
  name: "Tickets MCP",
  slug: "tickets-mcp",
  kind: "remote",
  listen_path: "/tickets-mcp/",
  auth_mode: "auth_token",
  auth_details: {},
  primitives: [],
  gateway_tags: { enabled: true, tags: ["edge-eu"] },
  dashboard_state: "pending_platform",
  origin: "submission",
  submission_id: 5,
  privacy_score: 20,
  is_active: false,
  brokerable: false,
  lock_version: 1,
  definition: "{}",
  group_ids: [],
  bundle: [],
};

const handoff = {
  server: pending,
  submission_id: 5,
  submitter: { name: "Member", email: "member@tyk.io", primary_contact: "member@tyk.io" },
  definition: { openapi: "3.0.3" },
  secrets_included: false,
  requested_gateway_tags: ["edge-eu"],
  instructions: ["Create the proxy on the Tyk Dashboard.", "Link it here."],
  candidates: [{ id: 12, name: "tickets mcp", listen_path: "/tickets-mcp/", tyk_api_id: "api-tickets-platform" }],
};

describe("MCPServerDetail handoff", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((path) => {
      if (path === "/mcp-servers/9") return Promise.resolve({ data: pending });
      if (path === "/mcp-servers/9/handoff") return Promise.resolve({ data: handoff });
      if (path === "/mcp-servers/9/handoff?include_secrets=true") return Promise.resolve({ data: { ...handoff, secrets_included: true } });
      if (path === "/tyk-connections/1/policies") return Promise.resolve({ data: [] });
      if (path === "/tyk-connections/1") return Promise.resolve({ data: { id: 1, name: "Catalogue", status: "active", effective_mode: "catalogue" } });
      if (path === "/groups") return Promise.resolve({ data: [] });
      return Promise.reject(new Error("unexpected " + path));
    });
    global.URL.createObjectURL = jest.fn(() => "blob:x");
    global.URL.revokeObjectURL = jest.fn();
  });

  it("shows the handoff, downloads the package with the credential and links a candidate", async () => {
    apiClient.post.mockResolvedValue({ data: { id: 12 } });
    render(
      <ThemeProvider theme={theme}>
        <MemoryRouter initialEntries={["/admin/mcp-servers/9"]}>
          <Routes>
            <Route path="/admin/mcp-servers/:id" element={<MCPServerDetail />} />
          </Routes>
        </MemoryRouter>
      </ThemeProvider>
    );
    expect(await screen.findByText(/Awaiting the platform team/)).toBeInTheDocument();
    expect(await screen.findByText(/Submitted by Member/)).toBeInTheDocument();
    expect(screen.getByText("Link it here.")).toBeInTheDocument();
    expect(screen.getByTestId("link-candidates")).toHaveTextContent("api-tickets-platform");

    fireEvent.click(screen.getByTestId("download-handoff-secrets"));
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/mcp-servers/9/handoff?include_secrets=true"));
    expect(await screen.findByText(/this download is audited/)).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("link-api-tickets-platform"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-servers/9/link", { tyk_api_id: "api-tickets-platform" }));
  });
});
