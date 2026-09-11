import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import Webhooks from "./Webhooks";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
  StyledTableHeaderCell: ({ children, sx, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, sx, ...props }) => <td {...props}>{children}</td>,
  StyledTableRow: ({ children, hover, sx, ...props }) => <tr {...props}>{children}</tr>,
}));

const theme = createTheme();

const renderPage = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <Webhooks />
      </MemoryRouter>
    </ThemeProvider>
  );

const healthyStatus = {
  available: true,
  enabled: true,
  bus_connected: true,
  worker_enabled: true,
  workers: 4,
  queue_depth: 2,
  retrying: 1,
  in_flight: 0,
  dead_lettered: 3,
  pending_targets: 1,
  dropped_events: 0,
};

const targets = [
  {
    id: "t-pending",
    name: "Ops Slack",
    url: "https://hooks.slack.com/services/abc",
    status: "pending",
    paused: false,
    topic_filters: ["system.llm.*"],
    template_preset: "slack",
    header_names: ["Authorization"],
    has_signing_secret: true,
    created_by_email: "admin@tyk.io",
    created_at: "2026-09-10T10:00:00Z",
    lock_version: 0,
    consecutive_failures: 0,
  },
  {
    id: "t-approved",
    name: "SIEM",
    url: "https://siem.example.com/ingest",
    status: "approved",
    paused: false,
    topic_filters: ["*"],
    template_preset: "standard",
    header_names: [],
    has_signing_secret: true,
    created_by_email: "admin@tyk.io",
    approved_by_email: "second@tyk.io",
    approved_at: "2026-09-10T11:00:00Z",
    created_at: "2026-09-10T10:00:00Z",
    last_success_at: "2026-09-10T12:00:00Z",
    lock_version: 3,
    consecutive_failures: 4,
  },
];

const presets = [
  { name: "standard", description: "Full envelope", body: "{}" },
  { name: "slack", description: "Slack message", body: "{}" },
  { name: "minimal", description: "Identifiers only", body: "{}" },
];

const mockApi = ({ status = healthyStatus, list = targets } = {}) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/webhooks/status") return Promise.resolve({ data: status });
    if (url === "/audit/status") return Promise.resolve({ data: { available: true, enabled: true } });
    if (url === "/webhooks/targets") {
      return Promise.resolve({ data: { targets: list, pending_count: list.filter((t) => t.status === "pending").length } });
    }
    if (url === "/webhooks/templates/presets") return Promise.resolve({ data: presets });
    if (url === "/webhooks/topics") return Promise.resolve({ data: { known: ["system.llm.created"], seen: [] } });
    if (url === "/webhooks/targets/t-approved") {
      return Promise.resolve({
        data: {
          target: list[1],
          stats: { succeeded: 12, dead_lettered: 1, queued: 0, retrying: 0, in_flight: 0, p50_ms: 40, p95_ms: 120 },
        },
      });
    }
    if (url === "/audit/resources/webhook_target/t-approved") {
      return Promise.resolve({
        data: { records: [{ id: 9, timestamp: "2026-09-10T11:00:00Z", action: "Approve Webhook Target", user: "second@tyk.io", status: 200 }], total: 1 },
      });
    }
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
  apiClient.post.mockImplementation((url) => {
    if (url.endsWith("/approve") || url.endsWith("/reject") || url.endsWith("/revoke") || url.endsWith("/pause") || url.endsWith("/resume")) {
      return Promise.resolve({ data: { ...targets[0], status: "approved" } });
    }
    if (url === "/webhooks/targets") {
      return Promise.resolve({ data: { target: { ...targets[0], id: "t-new", name: "New hook" }, signing_secret: "deadbeef".repeat(8) } });
    }
    if (url === "/webhooks/templates/preview") {
      return Promise.resolve({ data: { rendered: '{"id":"evt_sample"}', valid: true } });
    }
    if (url.endsWith("/test")) return Promise.resolve({ data: { delivery_id: "d-1" } });
    if (url.endsWith("/rotate-secret")) return Promise.resolve({ data: { signing_secret: "newsecret", previous_valid_until: "2026-09-11T11:00:00Z" } });
    return Promise.reject(new Error(`unexpected POST ${url}`));
  });
  apiClient.patch.mockResolvedValue({ data: { target: targets[1], repended: false } });
  apiClient.delete.mockResolvedValue({ data: {} });
};

beforeEach(() => {
  jest.clearAllMocks();
});

describe("Webhooks", () => {
  it("shows the enterprise upsell when the feature is unavailable", async () => {
    mockApi({ status: { available: false } });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/webhooks/targets", expect.anything());
  });

  it("explains when webhooks are disabled", async () => {
    mockApi({ status: { available: true, enabled: false } });
    renderPage();
    expect(await screen.findByText(/Webhooks are switched off/i)).toBeInTheDocument();
  });

  it("lists targets with status, pending badge and health", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("Ops Slack")).toBeInTheDocument();
    expect(screen.getByText("SIEM")).toBeInTheDocument();
    expect(screen.getByTestId("status-pending")).toBeInTheDocument();
    expect(screen.getByTestId("status-approved")).toBeInTheDocument();
    expect(screen.getByText("4 in a row")).toBeInTheDocument();
    expect(screen.getByText("Healthy")).toBeInTheDocument();
    expect(within(screen.getByTestId("pending-badge")).getByText("1")).toBeInTheDocument();
    expect(screen.getByText(/2 queued · 1 retrying · 3 dead-lettered/)).toBeInTheDocument();
  });

  it("warns when the node is not connected to the event bus", async () => {
    mockApi({ status: { ...healthyStatus, bus_connected: false } });
    renderPage();
    expect(await screen.findByText(/not connected to the event bus/i)).toBeInTheDocument();
  });

  it("approves a pending target through the confirmation dialog", async () => {
    mockApi();
    renderPage();
    await screen.findByText("Ops Slack");

    fireEvent.click(screen.getByLabelText("actions for Ops Slack"));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Approve" }));

    expect(await screen.findByText(/Approving allows AI Studio to POST/)).toBeInTheDocument();
    expect(screen.getByText(/Authorization/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "checked with security" } });
    fireEvent.click(screen.getByTestId("confirm-action"));

    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/webhooks/targets/t-pending/approve", { note: "checked with security" })
    );
    expect(await screen.findByText(/Ops Slack: approve done/)).toBeInTheDocument();
  });

  it("does not offer approve for an approved target but offers pause, test and revoke", async () => {
    mockApi();
    renderPage();
    await screen.findByText("SIEM");
    fireEvent.click(screen.getByLabelText("actions for SIEM"));
    expect(await screen.findByRole("menuitem", { name: "Pause" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Send test event" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Revoke" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Approve" })).not.toBeInTheDocument();
  });

  it("creates a target and shows the one-time secret", async () => {
    mockApi();
    renderPage();
    await screen.findByText("Ops Slack");

    fireEvent.click(screen.getByRole("button", { name: "New target" }));
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "New hook" } });
    fireEvent.change(screen.getByLabelText(/^URL/), { target: { value: "https://hooks.example.com/x" } });
    fireEvent.click(screen.getByRole("button", { name: "Create (pending approval)" }));

    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/webhooks/targets", expect.objectContaining({
      name: "New hook",
      url: "https://hooks.example.com/x",
      template_preset: "standard",
    })));
    expect(await screen.findByTestId("signing-secret")).toHaveTextContent("deadbeef");
  });

  it("previews the payload template from the form", async () => {
    mockApi();
    renderPage();
    await screen.findByText("Ops Slack");
    fireEvent.click(screen.getByRole("button", { name: "New target" }));
    fireEvent.click(await screen.findByRole("button", { name: "Preview" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/webhooks/templates/preview", expect.objectContaining({ template_preset: "standard" })));
    expect(await screen.findByTestId("template-preview")).toHaveTextContent("evt_sample");
  });

  it("expands a target to show statistics and audit history", async () => {
    mockApi();
    renderPage();
    await screen.findByText("SIEM");
    fireEvent.click(screen.getByTestId("target-row-t-approved"));
    expect(await screen.findByText(/12 succeeded · 1 dead-lettered/)).toBeInTheDocument();
    expect(await screen.findByTestId("target-audit-history")).toBeInTheDocument();
    expect(screen.getByText("Approve Webhook Target")).toBeInTheDocument();
  });

  it("falls back to the upsell when the API returns 403", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/webhooks/status") return Promise.resolve({ data: healthyStatus });
      if (url === "/audit/status") return Promise.resolve({ data: { available: false } });
      return Promise.reject({ response: { status: 403 } });
    });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
  });
});
