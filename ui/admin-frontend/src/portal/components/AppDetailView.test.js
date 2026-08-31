import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import AppDetailView from "./AppDetailView";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), delete: jest.fn() },
}));

jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="chart" /> }));

jest.mock("../../config", () => ({
  getConfig: () => ({ apiUrl: "http://localhost", proxyUrl: "http://localhost:9090" }),
}));

const pubClient = require("../../admin/utils/pubClient").default;

// The component uses `response.data` directly as the app object, and the
// accessible-* endpoints as plain arrays.
const appFixture = (credentialActive) => ({
  data: {
    id: "1",
    type: "apps",
    attributes: {
      name: "My App",
      description: "desc",
      credential: {
        key_id: "key-123",
        secret: "secret-abc",
        active: credentialActive,
      },
      llm_ids: [],
      datasource_ids: [],
      tool_ids: [],
      plugin_resources: [],
      monthly_budget: null,
    },
  },
});

const renderView = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/portal/apps/1"]}>
        <Routes>
          <Route path="/portal/apps/:id" element={<AppDetailView />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("AppDetailView credential state", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({ data: [] });
  });

  // A portal-created App's credential is minted inactive; an administrator has
  // to approve the App before the key works. The portal showed a secret you
  // could copy with nothing indicating it was not yet live, so the state was
  // discovered as a 401.
  it("says the credential is waiting for approval when it is inactive", async () => {
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/apps/1") return Promise.resolve(appFixture(false));
      return Promise.resolve({ data: [] });
    });

    renderView();

    await waitFor(() => {
      expect(screen.getByText("Waiting for approval")).toBeInTheDocument();
    });
    expect(screen.getByText(/401 Unauthorized/)).toBeInTheDocument();
    expect(screen.getByText("Pending approval")).toBeInTheDocument();
  });

  it("shows no pending notice once the credential is active", async () => {
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/apps/1") return Promise.resolve(appFixture(true));
      return Promise.resolve({ data: [] });
    });

    renderView();

    await waitFor(() => {
      expect(screen.getByText("Key ID:")).toBeInTheDocument();
    });
    expect(screen.queryByText("Waiting for approval")).not.toBeInTheDocument();
    expect(screen.queryByText("Pending approval")).not.toBeInTheDocument();
  });
});
