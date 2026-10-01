import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import EdgeGatewayList from "./EdgeGatewayList";

jest.mock("../../hooks/useNamespaces", () => ({
  __esModule: true,
  default: () => ({ getAvailableNamespaces: () => [] }),
}));

jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: { hub_spoke_multi_tenant: false } }),
}));

jest.mock("../../context/SyncStatusContext", () => ({
  useSyncStatus: () => ({ syncStatus: { data: [] }, getLastPushAt: () => undefined }),
}));

// The modals fetch on their own; they are closed and not under test here.
jest.mock("./PushConfigurationModal", () => () => null);
jest.mock("./RemoveEdgeModal", () => () => null);

jest.mock("../../services/edgeGatewayService", () => ({
  __esModule: true,
  default: {
    listEdgeGateways: jest.fn(),
    getEdgesInNamespace: jest.fn(),
    getConnectionStatus: () => ({ label: "Connected", color: "success" }),
    getSyncStatusDisplay: () => ({ label: "In Sync", color: "success" }),
    formatLastHeartbeat: () => "just now",
  },
}));
const edgeGatewayService = require("../../services/edgeGatewayService").default;

const edge = (edgeId, owner = {}) => ({
  id: edgeId,
  edgeId,
  namespace: "global",
  version: "1.0.0",
  syncStatus: "in_sync",
  ownerNodeId: "",
  ownerLabel: "",
  ownerLive: false,
  ...owner,
});

const renderList = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <EdgeGatewayList />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("EdgeGatewayList Held by column", () => {
  let edges;
  beforeEach(() => {
    // CRA's Jest config resets mock implementations before each test.
    edgeGatewayService.listEdgeGateways.mockImplementation(() => Promise.resolve({ data: edges, meta: {} }));
  });

  it("names the replica holding each edge when there are several", async () => {
    edges = [
      edge("edge-a", { ownerNodeId: "dash-1", ownerLabel: "dashboard", ownerLive: true }),
      edge("edge-b", { ownerNodeId: "mdcb-host-2", ownerLabel: "", ownerLive: true }),
      edge("edge-c"),
    ];
    renderList();

    expect(await screen.findByText("edge-a")).toBeInTheDocument();
    expect(screen.getByText("Held by")).toBeInTheDocument();
    const cells = screen.getAllByTestId("edge-held-by").map((c) => c.textContent);
    expect(cells).toEqual(["dashboard", "mdcb-host-2", "—"]);
  });

  it("is hidden while one replica holds every edge", async () => {
    edges = [
      edge("edge-a", { ownerNodeId: "dash-1", ownerLabel: "studio", ownerLive: true }),
      edge("edge-b", { ownerNodeId: "dash-1", ownerLabel: "studio", ownerLive: true }),
    ];
    renderList();

    expect(await screen.findByText("edge-a")).toBeInTheDocument();
    expect(screen.queryByText("Held by")).not.toBeInTheDocument();
    expect(screen.queryAllByTestId("edge-held-by")).toHaveLength(0);
  });
});
