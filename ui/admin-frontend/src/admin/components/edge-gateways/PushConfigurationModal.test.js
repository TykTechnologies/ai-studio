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
  },
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
        <PushConfigurationModal open onClose={() => {}} {...props} />
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

  it("starts the post-push polling after a successful push", async () => {
    edgeGatewayService.reloadAllEdges.mockResolvedValue({ operationId: "op-1" });
    renderModal();
    await screen.findByTestId("pending-changes-default");
    fireEvent.click(screen.getByRole("button", { name: /push configuration/i }));
    await waitFor(() => expect(mockSyncContext.notifyConfigPushed).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/successfully pushed/)).toBeInTheDocument();
    expect(screen.queryByTestId("pending-changes-preview")).not.toBeInTheDocument();
  });
});
