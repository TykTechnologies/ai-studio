import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import AppDetails from "./AppDetails";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

// Workspaces E4: the App page's actions are checked in the App's Workspace,
// and an App the caller cannot see (404) shows the not-found panel.

jest.mock("react-chartjs-2", () => ({ Line: () => null }));
jest.mock("../common/UsedBySection", () => () => null);
jest.mock("./AppGuardrails", () => () => null);
jest.mock("./AppTeamBudgetRow", () => () => null);
jest.mock("../../services/agentService", () => ({
  __esModule: true,
  default: { getAgents: jest.fn(), listAgents: jest.fn() },
}));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), patch: jest.fn(), post: jest.fn() },
  appToolAPI: { getAppTools: jest.fn() },
}));
jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: true }),
}));

const apiClient = require("../../utils/apiClient").default;
const { appToolAPI } = require("../../utils/apiClient");
const agentService = require("../../services/agentService").default;

const app = (workspaceId) => ({
  id: "7",
  attributes: {
    name: "Billing Copilot",
    user_id: 2,
    llm_ids: [],
    datasource_ids: [],
    tools: [],
    credential_id: 9,
    monthly_budget: 100,
    workspace_id: workspaceId,
  },
});

// Writes Apps and approves credentials in W1 (id 2) only; reads everywhere.
const scoped = {
  id: "9",
  attributes: {
    has_admin_access: true,
    rbac_enabled: true,
    permissions: ["apps:read", "credentials:read"],
    workspaces: [{ id: 2, name: "W1", permissions: ["apps:read", "apps:write", "credentials:write"] }],
  },
};

const renderDetails = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/apps/7"]}>
        <PermissionsProvider identity={scoped}>
          <Routes>
            <Route path="/admin/apps/:id" element={<AppDetails />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const mockApp = (result) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/apps/7") return result;
    if (url === "/credentials/9") return Promise.resolve({ data: { data: { id: "9", attributes: { key_id: "k", active: false } } } });
    if (url === "/users/2") return Promise.resolve({ data: { data: { id: "2", attributes: { name: "Dev" } } } });
    if (url.startsWith("/analytics/") && url !== "/analytics/proxy-logs-for-app") {
      return Promise.resolve({ data: { labels: [], datasets: [] } });
    }
    return Promise.resolve({ data: { data: [] }, headers: {} });
  });
};

describe("AppDetails Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    appToolAPI.getAppTools.mockResolvedValue({ data: { data: [] } });
    agentService.getAgents.mockResolvedValue({ data: [] });
    agentService.listAgents.mockResolvedValue({ data: [] });
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("offers Edit, Reset Budget and Approve for an App in the caller's Workspace", async () => {
    mockApp(Promise.resolve({ data: { data: app(2) } }));
    renderDetails();
    expect(await screen.findByRole("button", { name: "Edit app" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reset Budget" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Approve this App" })).toBeInTheDocument();
  });

  it("an App of another Workspace shows no Edit, Reset Budget or Approve", async () => {
    mockApp(Promise.resolve({ data: { data: app(3) } }));
    renderDetails();
    await screen.findByText("Proxy Logs");
    await screen.findByText("Credential Information");
    expect(screen.queryByRole("button", { name: "Edit app" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reset Budget" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve this App" })).not.toBeInTheDocument();
  });

  it("a 404 (an App in another Workspace looks missing) renders the not-found panel", async () => {
    mockApp(Promise.reject({ response: { status: 404, data: {} } }));
    renderDetails();
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });
});
