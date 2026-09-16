import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import AppMCPAccess from "./AppMCPAccess";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));

const pubClient = require("../../admin/utils/pubClient").default;

const weather = {
  id: 7,
  connection_id: 1,
  connection_name: "Prod",
  name: "Weather",
  slug: "weather",
  auth_mode: "auth_token",
  endpoint_url: "https://gw.example.com/weather/mcp",
  header_name: "Authorization",
  brokerable: true,
  grant_kind: "key",
  grant_open: true,
};

const oauthServer = {
  id: 8,
  connection_id: 1,
  connection_name: "Prod",
  name: "Tickets",
  slug: "tickets",
  auth_mode: "oauth21",
  endpoint_url: "https://gw.example.com/tickets/mcp",
  prm: { authorization_servers: ["https://auth.example.com"], scopes_supported: ["tickets:read"] },
  brokerable: false,
  grant_kind: "oauth",
  grant_open: true,
};

const summary = (overrides = {}) => ({
  data: {
    servers: [weather, oauthServer],
    credentials: [],
    connections: [{ connection_id: 1, connection_name: "Prod", can_mint: true }],
    ...overrides,
  },
});

describe("AppMCPAccess", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("mints a key and reveals it exactly once", async () => {
    pubClient.get.mockResolvedValue(summary());
    pubClient.post.mockResolvedValue({
      data: {
        key: "org1plaintextkey1",
        credential: { id: "cred-1", status: "active", connection_id: 1, expires_at: null },
        servers: [weather],
        skipped: [{ id: 8, name: "Tickets", reason: "not brokerable" }],
      },
    });
    render(<AppMCPAccess appId="1" credentialActive />);

    await screen.findByTestId("mcp-connection-1");
    // OAuth server shows its authorization server rather than a key control.
    expect(screen.getByTestId("mcp-access-server-8")).toHaveTextContent("https://auth.example.com");
    expect(screen.getByTestId("mcp-access-server-8")).toHaveTextContent("tickets:read");

    // After the mint the summary reports the live credential.
    pubClient.get.mockResolvedValue(
      summary({
        credentials: [{ id: "cred-1", status: "active", connection_id: 1, key_hint: "key1", minted_at: "2026-09-16T10:00:00Z" }],
        connections: [{ connection_id: 1, connection_name: "Prod", can_mint: false, credential_id: "cred-1" }],
      })
    );
    fireEvent.click(screen.getByTestId("mint-1"));

    const dialog = await screen.findByTestId("reveal-key-dialog");
    expect(pubClient.post).toHaveBeenCalledWith("/common/apps/1/mcp/credentials", { connection_id: 1 });
    expect(screen.getByTestId("revealed-key")).toHaveTextContent("org1plaintextkey1");
    expect(dialog).toHaveTextContent("mcp-remote");
    expect(screen.getByTestId("reveal-skipped")).toHaveTextContent("Tickets (not brokerable)");

    fireEvent.click(screen.getByTestId("reveal-close"));
    await waitFor(() => expect(screen.queryByTestId("reveal-key-dialog")).not.toBeInTheDocument());
    // The key never reappears on the page.
    expect(screen.queryByText(/org1plaintextkey1/)).not.toBeInTheDocument();
    expect(await screen.findByTestId("credential-status-active")).toBeInTheDocument();
    expect(screen.getByTestId("rotate-1")).toBeInTheDocument();
    expect(screen.queryByTestId("mint-1")).not.toBeInTheDocument();
  });

  it("explains why a key cannot be minted yet", async () => {
    pubClient.get.mockResolvedValue(
      summary({ connections: [{ connection_id: 1, connection_name: "Prod", can_mint: false, reason: "the App has not been approved yet" }] })
    );
    render(<AppMCPAccess appId="1" credentialActive={false} />);
    expect(await screen.findByTestId("mint-reason-1")).toHaveTextContent("not been approved");
    expect(screen.queryByTestId("mint-1")).not.toBeInTheDocument();
  });

  it("revokes after confirmation and surfaces API errors", async () => {
    pubClient.get.mockResolvedValue(
      summary({
        credentials: [{ id: "cred-1", status: "active", connection_id: 1, key_hint: "key1", drift: "pending_widen" }],
        connections: [{ connection_id: 1, connection_name: "Prod", can_mint: false, credential_id: "cred-1" }],
      })
    );
    pubClient.post.mockRejectedValue({ response: { data: { detail: "Dashboard unreachable" } } });
    render(<AppMCPAccess appId="1" credentialActive />);

    await screen.findByTestId("revoke-1");
    expect(screen.getByText("Change pending")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("revoke-1"));
    fireEvent.click(await screen.findByTestId("confirm-action"));
    await waitFor(() => expect(pubClient.post).toHaveBeenCalledWith("/common/apps/1/mcp/credentials/cred-1/revoke", {}));
    expect(await screen.findByTestId("mcp-access-error")).toHaveTextContent("Dashboard unreachable");
  });
});
