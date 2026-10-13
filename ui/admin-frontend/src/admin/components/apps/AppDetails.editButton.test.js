import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import AppDetails from "./AppDetails";

// #236: Edit sat below the proxy log browser, a long scroll away. It now
// sits in the page header, like the other detail pages, for users who may
// edit apps.

jest.mock("react-chartjs-2", () => ({ Line: () => null }));
jest.mock("../common/UsedBySection", () => () => null);
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
let mockGranted = () => true;
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: () => ({ can: (p) => mockGranted(p), canAny: () => true, canAll: () => true }),
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const apiClient = require("../../utils/apiClient").default;
const { appToolAPI } = require("../../utils/apiClient");
const agentService = require("../../services/agentService").default;

const app = {
  id: "7",
  attributes: { name: "Billing Copilot", user_id: 2, llm_ids: [], datasource_ids: [], tools: [] },
};

const renderDetails = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/apps/7"]}>
        <Routes>
          <Route path="/admin/apps/:id" element={<AppDetails />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("AppDetails: Edit in the header (#236)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGranted = () => true;
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps/7") return Promise.resolve({ data: { data: app } });
      if (url === "/users/2") return Promise.resolve({ data: { data: { id: "2", attributes: { name: "Dev" } } } });
      if (url.startsWith("/analytics/") && url !== "/analytics/proxy-logs-for-app") {
        return Promise.resolve({ data: { labels: [], datasets: [] } });
      }
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
    appToolAPI.getAppTools.mockResolvedValue({ data: { data: [] } });
    agentService.getAgents.mockResolvedValue({ data: [] });
    agentService.listAgents.mockResolvedValue({ data: [] });
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("puts Edit app above the proxy log browser and opens the edit form", async () => {
    renderDetails();
    const edit = await screen.findByRole("button", { name: "Edit app" });
    const logs = await screen.findByText("Proxy Logs");
    // eslint-disable-next-line no-bitwise
    expect(edit.compareDocumentPosition(logs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Edit app" })).toHaveLength(1);

    fireEvent.click(edit);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps/edit/7");
  });

  it("is hidden from users who cannot edit apps", async () => {
    mockGranted = (p) => p !== "apps:write";
    renderDetails();
    await screen.findByText("Proxy Logs");
    expect(screen.queryByRole("button", { name: "Edit app" })).not.toBeInTheDocument();
  });
});
