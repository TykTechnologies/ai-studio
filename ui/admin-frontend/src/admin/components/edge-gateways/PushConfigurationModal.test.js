import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import PushConfigurationModal from "./PushConfigurationModal";

jest.mock("../../hooks/useNamespaces", () => ({
  __esModule: true,
  default: () => ({
    getAvailableNamespaces: () => [
      { name: "default", edgeCount: 1 },
      { name: "team-a", edgeCount: 2 },
    ],
  }),
}));

// Enterprise: namespace selection is available.
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: { hub_spoke_multi_tenant: true } }),
}));

const mockSyncContext = {
  refreshSyncStatus: jest.fn(),
  notifyConfigPushed: jest.fn(),
  syncStatus: {
    has_pending: true,
    data: [
      { namespace: "default", pending_count: 1, stale_count: 0 },
      { namespace: "team-a", pending_count: 0, stale_count: 0 },
    ],
  },
};
jest.mock("../../context/SyncStatusContext", () => ({
  useSyncStatus: () => mockSyncContext,
}));

jest.mock("../../services/edgeGatewayService", () => ({
  __esModule: true,
  default: {
    getPendingChanges: jest.fn(),
    reloadAllEdges: jest.fn(),
    triggerConfigurationReload: jest.fn(),
    getPushProgress: jest.fn(),
  },
  PUSH_FINAL_STATUSES: ["succeeded", "succeeded_with_warnings", "partially_failed", "failed", "expired"],
}));
const edgeGatewayService = require("../../services/edgeGatewayService").default;

const pendingFor = (namespace) => {
  if (namespace === "default") {
    return Promise.resolve({
      namespace,
      lastPushAt: "2026-09-14T13:12:00Z",
      total: 3,
      changes: [
        { type: "llm", id: 1, name: "OpenAI", change: "updated", at: new Date().toISOString() },
        { type: "app", id: 7, name: "Support bot", change: "created", at: new Date().toISOString() },
        { type: "app", id: 8, name: "Old bot", change: "deleted", at: new Date().toISOString() },
      ],
    });
  }
  return Promise.resolve({ namespace, lastPushAt: "2026-09-14T12:00:00Z", total: 0, changes: [] });
};

const renderModal = (props = {}) =>
  render(
    <MemoryRouter>
      <ThemeProvider theme={testTheme}>
        <PushConfigurationModal open onClose={() => {}} pollIntervalMs={5} {...props} />
      </ThemeProvider>
    </MemoryRouter>
  );

describe("PushConfigurationModal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    edgeGatewayService.getPendingChanges.mockImplementation(pendingFor);
  });

  // On Enterprise this opened defaulted to "Specific Namespace" with nothing
  // selected, so the submit button was already disabled with no indication of
  // why and the user had to work out that a namespace still had to be chosen.
  it("opens in a state it can actually submit", () => {
    renderModal();
    const push = screen.getByRole("button", { name: /push configuration/i });
    expect(push).not.toBeDisabled();
  });

  it("previews what will be pushed, grouped by type, with links", async () => {
    renderModal();
    // One block per namespace when pushing everything.
    expect(await screen.findByTestId("pending-changes-default")).toBeInTheDocument();
    expect(screen.getByText(/3 changes since the last push at/)).toBeInTheDocument();
    expect(screen.getByText("LLM providers")).toBeInTheDocument();
    expect(screen.getByText("Apps")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "OpenAI" })).toHaveAttribute("href", "/admin/llms/1");
    expect(screen.getByRole("link", { name: "Support bot" })).toHaveAttribute("href", "/admin/apps/7");
    // Deleted objects have no page to link to.
    expect(screen.queryByRole("link", { name: "Old bot" })).not.toBeInTheDocument();
    expect(screen.getByText("Old bot")).toBeInTheDocument();
    expect(screen.getAllByText("updated").length).toBeGreaterThan(0);
    expect(edgeGatewayService.getPendingChanges).toHaveBeenCalledWith("default");
    expect(edgeGatewayService.getPendingChanges).toHaveBeenCalledWith("team-a");
    expect(screen.getByRole("button", { name: /push configuration/i })).toBeInTheDocument();
  });

  it("says 'Push anyway' when nothing has changed", async () => {
    renderModal();
    fireEvent.click(screen.getByLabelText("Specific Namespace"));
    fireEvent.mouseDown(screen.getByLabelText(/select namespace/i));
    fireEvent.click(await screen.findByRole("option", { name: /team-a/ }));
    expect(await screen.findByText(/Nothing has changed since the last push/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Push anyway" })).not.toBeDisabled();
  });

  it("shows 'and N more' when the list is capped", async () => {
    edgeGatewayService.getPendingChanges.mockImplementation((ns) =>
      Promise.resolve({
        namespace: ns,
        lastPushAt: null,
        total: 5,
        changes: [{ type: "tool", id: 1, name: "Search", change: "created", at: null }],
      })
    );
    renderModal();
    expect(await screen.findAllByText("and 4 more")).toHaveLength(2);
    expect(screen.getAllByText("5 changes (never pushed)")).toHaveLength(2);
  });

  it("shows a loading state and then an error without blocking the push", async () => {
    let reject;
    edgeGatewayService.getPendingChanges.mockImplementation(
      () => new Promise((_, r) => { reject = r; })
    );
    renderModal();
    expect(screen.getByTestId("pending-changes-loading-default")).toBeInTheDocument();
    reject(new Error("boom"));
    expect(await screen.findAllByText(/Could not load the list of changes \(boom\)/)).not.toHaveLength(0);
    expect(screen.getByRole("button", { name: /push configuration/i })).not.toBeDisabled();
  });

  const started = (overrides = {}) => ({
    operationId: "push-1",
    scope: "all",
    targetEdges: ["edge-a", "edge-b"],
    status: "in_progress",
    progress: 0,
    message: "Push recorded for 2 edge(s).",
    counts: {},
    warnings: [],
    skipped: [],
    targets: [],
    edges: null,
    deadlineAt: new Date(Date.now() + 300000).toISOString(),
    ...overrides,
  });

  const clickPush = async () => {
    await screen.findByTestId("pending-changes-default");
    fireEvent.click(screen.getByRole("button", { name: /push configuration/i }));
  };

  // Starting a push is not delivering it: the dialog follows the push until
  // every edge has answered, and only then says it worked.
  it("follows the push until the edges have loaded it", async () => {
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started());
    edgeGatewayService.getPushProgress
      .mockResolvedValueOnce(started({
        progress: 50,
        message: "Pushing to 2 edge(s): 1 updated, 1 reloading",
        edges: [
          { edgeId: "edge-a", status: "succeeded", message: "", warning: "", attempts: 1, maxAttempts: 3 },
          { edgeId: "edge-b", status: "sent", phase: "PULL_STARTED", message: "", warning: "", attempts: 1, maxAttempts: 3 },
        ],
      }))
      .mockResolvedValue(started({
        status: "succeeded",
        progress: 100,
        message: "All 2 edge(s) loaded the configuration.",
        edges: [
          { edgeId: "edge-a", status: "succeeded", message: "", warning: "", attempts: 1, maxAttempts: 3 },
          { edgeId: "edge-b", status: "succeeded", message: "", warning: "", attempts: 2, maxAttempts: 3 },
        ],
      }));
    renderModal();
    await clickPush();

    // Recorded, not yet a success.
    expect(await screen.findByText("Pushing configuration")).toBeInTheDocument();
    expect(screen.queryByText(/successfully pushed/i)).not.toBeInTheDocument();
    await waitFor(() => expect(mockSyncContext.notifyConfigPushed).toHaveBeenCalledTimes(1));

    expect(await screen.findByText("Configuration pushed")).toBeInTheDocument();
    expect(screen.getByText("All 2 edge(s) loaded the configuration.")).toBeInTheDocument();
    expect(screen.getByTestId("push-edge-edge-b")).toHaveTextContent("Updated");
    expect(screen.getByTestId("push-edge-edge-b")).toHaveTextContent("attempt 2 of 3");
    expect(mockSyncContext.refreshSyncStatus).toHaveBeenCalled();
    expect(screen.queryByTestId("pending-changes-preview")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();

    // It stops polling once the push is settled.
    const calls = edgeGatewayService.getPushProgress.mock.calls.length;
    await new Promise((r) => setTimeout(r, 50));
    expect(edgeGatewayService.getPushProgress.mock.calls.length).toBe(calls);
  });

  it("says which edges failed and why", async () => {
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started());
    edgeGatewayService.getPushProgress.mockResolvedValue(started({
      status: "partially_failed",
      progress: 100,
      message: "Push finished: 1 updated, 1 failed",
      edges: [
        { edgeId: "edge-a", status: "succeeded", message: "", warning: "", attempts: 1, maxAttempts: 3 },
        { edgeId: "edge-b", status: "failed", message: "Failed to update SQLite: disk full", warning: "", attempts: 1, maxAttempts: 3 },
      ],
    }));
    renderModal();
    await clickPush();

    expect(await screen.findByText("Some edge gateways did not update")).toBeInTheDocument();
    const failed = screen.getByTestId("push-edge-edge-b");
    expect(failed).toHaveTextContent("Failed");
    expect(failed).toHaveTextContent("Failed to update SQLite: disk full");
  });

  it("warns about edges that are not connected, and ones left out", async () => {
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started({
      warnings: ["1 of 2 edge(s) are not connected to the control plane; the push waits up to 5m0s for them to reconnect."],
      skipped: [{ edgeId: "edge-old", namespace: "default", reason: "offline since 2026-09-28T00:00:00Z" }],
      skippedTotal: 3,
    }));
    edgeGatewayService.getPushProgress.mockResolvedValue(started({
      edges: [
        { edgeId: "edge-a", status: "pending", message: "", warning: "", waitingReason: "no recent heartbeat", attempts: 0, maxAttempts: 3 },
      ],
    }));
    renderModal();
    await clickPush();

    expect(await screen.findByTestId("push-warnings")).toHaveTextContent("not connected to the control plane");
    expect(screen.getByTestId("push-skipped")).toHaveTextContent("edge-old: offline since");
    expect(screen.getByTestId("push-skipped")).toHaveTextContent("and 2 more");
    expect(await screen.findByTestId("push-edge-edge-a")).toHaveTextContent("Waiting: no recent heartbeat");
  });

  it("reports the outcome from the summary when per-edge results are unavailable (Community Edition)", async () => {
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started());
    edgeGatewayService.getPushProgress.mockResolvedValue(started({
      status: "expired",
      progress: 100,
      message: "Push finished: 1 updated, 1 timed out",
      edges: null,
    }));
    renderModal();
    await clickPush();

    expect(await screen.findByText("The push timed out")).toBeInTheDocument();
    expect(screen.getByText("Push finished: 1 updated, 1 timed out")).toBeInTheDocument();
    expect(screen.getByText(/Per-edge results are available in Enterprise Edition/)).toBeInTheDocument();
  });

  it("keeps polling through a failed progress check", async () => {
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started());
    edgeGatewayService.getPushProgress
      .mockRejectedValueOnce(new Error("Network Error"))
      .mockResolvedValue(started({ status: "succeeded", progress: 100, message: "All 2 edge(s) loaded the configuration.", edges: [] }));
    renderModal();
    await clickPush();

    expect(await screen.findByText("Configuration pushed")).toBeInTheDocument();
  });

  it("shows why a push could not start", async () => {
    edgeGatewayService.reloadAllEdges.mockRejectedValue(new Error('no edges to push to: no edges are registered in namespace "default"'));
    renderModal();
    await clickPush();

    expect(await screen.findByText(/no edges are registered in namespace "default"/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /push configuration/i })).toBeInTheDocument();
    expect(edgeGatewayService.getPushProgress).not.toHaveBeenCalled();
  });

  // Every edge offline for longer than the push waits (409): the reason is
  // at the top of the dialog, the dialog no longer claims it will push to
  // every namespace, and Push stays disabled for that target (L7).
  it("puts 'nothing to push to' at the top and disables Push for that target", async () => {
    const err = new Error("no edges to push to: the 1 edge(s) in any namespace have been offline for more than 5m0s");
    err.status = 409;
    edgeGatewayService.reloadAllEdges.mockRejectedValue(err);
    renderModal();
    expect(screen.getByText(/will push configuration to all 2 namespaces/)).toBeInTheDocument();
    await clickPush();

    const alert = await screen.findByTestId("push-error");
    expect(alert).toHaveTextContent("Nothing to push to");
    expect(alert).toHaveTextContent("have been offline for more than 5m0s");
    // Above everything else in the dialog, not below the preview.
    const intro = screen.getByText(/Push the latest configuration to edge gateways/);
    expect(alert.compareDocumentPosition(intro) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const preview = screen.getByTestId("pending-changes-default");
    expect(alert.compareDocumentPosition(preview) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    expect(screen.queryByText(/will push configuration to all 2 namespaces/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /push configuration/i })).toBeDisabled();
    expect(screen.getByText(/No edge gateway can receive this push/)).toBeInTheDocument();

    // Another target may have edges to push to.
    fireEvent.click(screen.getByLabelText("Specific Namespace"));
    fireEvent.mouseDown(screen.getByLabelText(/select namespace/i));
    fireEvent.click(await screen.findByRole("option", { name: /default/ }));
    expect(screen.queryByTestId("push-error")).not.toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /push configuration/i })).not.toBeDisabled();
  });

  it("leaves Push enabled after other errors", async () => {
    const err = new Error("The push could not be recorded or read; see the server log.");
    err.status = 500;
    edgeGatewayService.reloadAllEdges.mockRejectedValue(err);
    renderModal();
    await clickPush();

    expect(await screen.findByTestId("push-error")).toHaveTextContent("see the server log");
    expect(screen.getByRole("button", { name: /push configuration/i })).not.toBeDisabled();
  });

  // The warning from starting a push ("... waits up to 5m0s for them to
  // reconnect") stands only while an edge is still waiting to connect; once
  // the edge has its push (reloading) it is gone (M3).
  it("drops the not-connected warning once no edge is waiting for a connection", async () => {
    const waitWarning = "1 of 1 edge(s) are not connected to the control plane; the push waits up to 5m0s for them to reconnect.";
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started({
      targetEdges: ["edge-a"],
      warnings: [waitWarning],
      targets: [{ edgeId: "edge-a", namespace: "default", reachable: false, reason: "not connected to any control-plane replica" }],
    }));
    const edge = (over) => ({ edgeId: "edge-a", message: "", warning: "", attempts: 0, maxAttempts: 3, ...over });
    edgeGatewayService.getPushProgress
      .mockResolvedValueOnce(started({ edges: [edge({ status: "pending", reachable: false, waitingReason: "not connected to any control-plane replica" })] }))
      .mockResolvedValue(started({ edges: [edge({ status: "sent", phase: "PULL_STARTED", attempts: 1 })] }));
    renderModal();
    await clickPush();

    expect(await screen.findByTestId("push-warnings")).toHaveTextContent("waits up to 5m0s");
    expect(screen.getByText(/Edges that are not connected are waited for/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("push-edge-edge-a")).toHaveTextContent("Reloading"));
    expect(screen.queryByTestId("push-warnings")).not.toBeInTheDocument();
    expect(screen.queryByText(/Edges that are not connected are waited for/)).not.toBeInTheDocument();
    expect(screen.getByText(/The push carries on if you close this dialog/)).toBeInTheDocument();
  });

  it("keeps the not-connected warning while something is pending (Community Edition)", async () => {
    const waitWarning = "1 of 2 edge(s) are not connected to the control plane; the push waits up to 5m0s for them to reconnect.";
    edgeGatewayService.reloadAllEdges.mockResolvedValue(started({ warnings: [waitWarning] }));
    edgeGatewayService.getPushProgress
      .mockResolvedValueOnce(started({ counts: { pending: 1, sent: 1 }, edges: null }))
      .mockResolvedValue(started({ counts: { sent: 2 }, edges: null }));
    renderModal();
    await clickPush();

    expect(await screen.findByTestId("push-warnings")).toHaveTextContent("waits up to 5m0s");
    await waitFor(() => expect(screen.queryByTestId("push-warnings")).not.toBeInTheDocument());
  });
});
