import React from "react";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import testTheme from "../../admin/utils/testTheme";
import PortalDashboard from "./PortalDashboard";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../../config", () => ({ getConfig: () => ({}) }));
jest.mock("../../admin/hooks/useSystemFeatures", () => () => ({
  features: { feature_portal: true, feature_gateway: true },
  loading: false,
}));
jest.mock("../../admin/hooks/useUserEntitlements", () => () => ({
  userName: "Dev One",
  uiOptions: { show_portal: true },
  userEntitlements: {},
  loading: false,
}));
jest.mock("../../components/common/Icon", () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const pubClient = require("../../admin/utils/pubClient").default;

const item = (type, id, attributes) => ({ type, id: String(id), attributes });

const catalog = {
  data: [
    item("llm", 2, { name: "Bedrock Claude", short_description: "Long context", kind: "bedrock", privacy_score: 80, created_at: "2026-09-10T00:00:00Z", catalogs: [] }),
    item("datasource", 3, { name: "Docs index", short_description: "Docs", kind: "pgvector", privacy_score: 20, created_at: "2026-09-05T00:00:00Z", catalogs: [] }),
    item("llm", 1, { name: "Acme OpenAI", short_description: "Fast", kind: "openai", privacy_score: 40, created_at: "2026-09-01T00:00:00Z", catalogs: [] }),
  ],
  meta: {
    total: 3,
    page: 1,
    page_size: 6,
    total_pages: 1,
    counts: { llm: 2, datasource: 1, tool: 0, plugin_resource: 0 },
    kinds: [
      { type: "llm", kind: "bedrock", label: "AWS Bedrock", count: 1 },
      { type: "llm", kind: "openai", label: "OpenAI", count: 1 },
      { type: "datasource", kind: "pgvector", count: 1 },
    ],
    catalogs: [],
    resource_types: [],
  },
};

const apps = {
  data: [
    { id: "1", attributes: { name: "Support bot", description: "Answers tickets", llm_ids: [1], datasource_ids: [3], tool_ids: [], is_active: true, credential_active: true, monthly_budget: 100 } },
    { id: "2", attributes: { name: "Draft app", description: "", llm_ids: [2], datasource_ids: [], tool_ids: [], is_active: true, credential_active: false, monthly_budget: null } },
  ],
};

const usage = {
  data: {
    1: { app_id: "1", current_spend: 42.5, monthly_budget: 100, percentage: 42.5, last_access_at: new Date(Date.now() - 3 * 3600 * 1000).toISOString(), requests_30d: 17 },
    2: { app_id: "2", current_spend: 0, monthly_budget: null, percentage: null, last_access_at: null, requests_30d: 0 },
  },
  spend_tracked: true,
};

const LocationProbe = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
};

const renderDashboard = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/portal/dashboard"]}>
        <Routes>
          <Route path="/portal/dashboard" element={<PortalDashboard />} />
          <Route path="*" element={<div>elsewhere</div>} />
        </Routes>
        <LocationProbe />
      </MemoryRouter>
    </ThemeProvider>
  );

const mockApi = ({ appsResponse = apps, usageResponse = usage, catalogResponse = catalog } = {}) => {
  pubClient.get.mockImplementation((url) => {
    if (url === "/common/apps") return Promise.resolve({ data: appsResponse });
    if (url === "/common/apps/usage-summary") return Promise.resolve({ data: usageResponse });
    if (url === "/common/catalog") return Promise.resolve({ data: catalogResponse });
    return Promise.reject(new Error(`unexpected ${url}`));
  });
};

// The landing page was one hero card with a button (UX review F-09). It now
// answers "how are my apps doing?" and "what can I build with?".
describe("PortalDashboard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockApi();
  });

  it("greets the user and lists their apps with status, budget and last access", async () => {
    renderDashboard();
    expect(await screen.findByRole("heading", { level: 1, name: "Hi Dev One" })).toBeInTheDocument();

    const table = await screen.findByRole("table", { name: "Your apps" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    // Most recently used first.
    expect(rows[0]).toHaveTextContent("Support bot");
    expect(within(rows[0]).getByTestId("app-status")).toHaveTextContent("Active");
    expect(within(rows[0]).getByTestId("budget-cell")).toHaveTextContent("$42.50 of $100.00");
    // MUI rounds the progress value for the accessibility attribute.
    expect(within(rows[0]).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "43");
    expect(within(rows[0]).getByTestId("last-access")).toHaveTextContent("3h ago");
    expect(rows[0]).toHaveTextContent("17 requests in 30 days");
    expect(rows[0]).toHaveTextContent("1 LLM provider, 1 data source");

    expect(within(rows[1]).getByTestId("app-status")).toHaveTextContent("Awaiting approval");
    expect(within(rows[1]).getByTestId("budget-cell")).toHaveTextContent("No limit");
    expect(within(rows[1]).getByTestId("last-access")).toHaveTextContent("Never");

    expect(screen.getByRole("link", { name: /All apps \(2\)/ })).toHaveAttribute("href", "/portal/apps");
  });

  it("shows counts per type and the newest assets, linking on to browse", async () => {
    renderDashboard();
    const counts = await screen.findByTestId("overview-counts");
    expect(pubClient.get).toHaveBeenCalledWith("/common/catalog", { params: { page_size: "6", sort: "newest" } });
    expect(within(counts).getByRole("link", { name: /2\s*LLM providers/ })).toHaveAttribute("href", "/portal/catalog/llms");
    expect(within(counts).getByRole("link", { name: /1\s*Data source/ })).toHaveAttribute("href", "/portal/catalog/datasources");
    expect(within(counts).queryByText(/Tools/)).not.toBeInTheDocument();

    const recent = screen.getByTestId("overview-recent");
    const names = within(recent).getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
    expect(names).toEqual(["Bedrock Claude", "Docs index", "Acme OpenAI"]);
    expect(screen.getByRole("link", { name: /Browse all/ })).toHaveAttribute("href", "/portal/catalog");
  });

  it("sends a search to the browse page", async () => {
    renderDashboard();
    await screen.findByTestId("overview-counts");
    const input = screen.getByPlaceholderText("Search LLM providers, data sources and tools");
    fireEvent.change(input, { target: { value: "claude" } });
    fireEvent.submit(input.closest("form"));
    expect(screen.getByTestId("location")).toHaveTextContent("/portal/catalog?q=claude");
  });

  it("invites a first app and explains an empty catalog", async () => {
    mockApi({
      appsResponse: { data: [] },
      usageResponse: { data: {}, spend_tracked: false },
      catalogResponse: { data: [], meta: { total: 0, page: 1, page_size: 6, total_pages: 1, counts: {}, kinds: [], catalogs: [], resource_types: [] } },
    });
    renderDashboard();
    expect(await screen.findByText("Create your first app")).toBeInTheDocument();
    expect(screen.getByTestId("overview-empty-catalog")).toBeInTheDocument();
    // The title bar has its own Create app; this is the empty state's.
    fireEvent.click(within(screen.getByTestId("overview-apps")).getByRole("button", { name: "Create app" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/portal/app/new");
  });

  it("shows the budget without spend when spending is not tracked", async () => {
    mockApi({ usageResponse: { ...usage, spend_tracked: false } });
    renderDashboard();
    const table = await screen.findByRole("table", { name: "Your apps" });
    const cells = within(table).getAllByTestId("budget-cell");
    expect(cells[0]).toHaveTextContent("$100.00 / month");
    expect(within(table).queryByRole("progressbar")).not.toBeInTheDocument();
  });
});
