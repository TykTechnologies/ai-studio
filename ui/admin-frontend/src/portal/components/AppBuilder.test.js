import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../admin/utils/testTheme";
import AppBuilder from "./AppBuilder";
import {
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../../components/unsaved-changes";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const pubClient = require("../../admin/utils/pubClient").default;

const llms = [
  { id: "1", attributes: { name: "OpenAI" } },
  { id: "2", attributes: { name: "Anthropic" } },
];
const dataSources = [{ id: "3", attributes: { name: "Docs" } }];
const tools = [{ id: "4", attributes: { name: "Weather" } }];
// Tyk-managed MCP servers from the unified catalog: only key-backed ones
// can be bound to an App; the OAuth one is reached directly.
const mcpServers = [
  { id: "12", type: "mcp_server", attributes: { name: "Weather MCP", brokerable: true, access_granted_via_app: true } },
  { id: "13", type: "mcp_server", attributes: { name: "Tickets MCP (OAuth)", brokerable: false, access_granted_via_app: false } },
];
const resourceTypes = [
  { plugin_id: 3, slug: "vector-db", name: "Vector DBs", instances: [{ id: "i1", name: "Pinecone" }] },
  // Catalog-style type: nothing here is unlocked by an app credential, except
  // one instance the plugin explicitly opted in.
  {
    plugin_id: 9,
    slug: "agent",
    name: "Agents",
    access_granted_via_app: false,
    instances: [
      { id: "a1", name: "Agent One", access_granted_via_app: false },
      { id: "a2", name: "Proxied Agent", access_granted_via_app: true },
    ],
  },
  { plugin_id: 9, slug: "prompt", name: "Prompts", access_granted_via_app: false, instances: [{ id: "p1", name: "Tone", access_granted_via_app: false }] },
];

const DirtyProbe = () => {
  const { isDirty } = useUnsavedChanges();
  return <span data-testid="registry-dirty">{String(isDirty)}</span>;
};

const renderBuilder = ({ path = "/portal/apps/new" } = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <UnsavedChangesProvider>
          <AppBuilder />
          <DirtyProbe />
        </UnsavedChangesProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const picker = () => screen.getByTestId("app-access-picker");
// The mocked Icon renders its name as text, so read the label on its own.
const tabLabel = (el) => el.querySelector("[data-testid=app-access-tab-label]").textContent;
const tab = (name) => within(picker()).getAllByRole("tab").find((el) => tabLabel(el) === name);
const tabNames = () => within(picker()).getAllByRole("tab").map(tabLabel);
const openTab = (name) => fireEvent.click(tab(name));
const options = () =>
  within(screen.getByTestId("app-access-options")).queryAllByRole("button").map((el) => el.getAttribute("aria-label"));
const addFromPicker = (name) =>
  fireEvent.click(within(screen.getByTestId("app-access-options")).getByRole("button", { name: `Add ${name}` }));
const requested = (groupKey) => {
  const group = screen.queryByTestId(`requested-access-${groupKey}`);
  return group
    ? within(group).getAllByTestId("requested-access-item").map((el) => el.querySelector(".MuiListItemText-primary").textContent)
    : [];
};

describe("AppBuilder access picker, name default and commit semantics", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/accessible-datasources") return Promise.resolve({ data: dataSources });
      if (url === "/common/accessible-llms") return Promise.resolve({ data: llms });
      if (url === "/common/accessible-tools") return Promise.resolve({ data: tools });
      if (url === "/common/accessible-plugin-resources") return Promise.resolve({ data: { data: resourceTypes } });
      if (url === "/common/catalog") return Promise.resolve({ data: { data: mcpServers } });
      return Promise.resolve({ data: [] });
    });
    // POST /common/apps answers with the serialized app itself, not a { data } envelope.
    pubClient.post.mockResolvedValue({ data: { type: "apps", id: "77", attributes: {} } });
  });

  // One list per asset type made the form a wall of dropdowns when an app
  // needs one thing; types are now a rail and only the chosen one is listed.
  it("starts empty, with one tab per grantable asset type and nothing requested", async () => {
    renderBuilder();
    await waitFor(() => expect(picker()).toBeTruthy());

    expect(screen.getByRole("textbox", { name: /App Name/ })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: /App Name/ })).toBeRequired();
    // Prompts has nothing an app credential unlocks, so it has no tab.
    expect(tabNames()).toEqual(["LLM providers", "Data sources", "Tools", "MCP servers", "Vector DBs", "Agents"]);
    expect(tab("LLM providers")).toHaveAttribute("aria-selected", "true");
    expect(options()).toEqual(["Add OpenAI", "Add Anthropic"]);
    expect(screen.getByTestId("requested-access-empty")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create App" })).toBeDisabled();
  });

  it("opens on a ?llm= preselection without dirtying the form, and submits what was requested", async () => {
    renderBuilder({ path: "/portal/apps/new?llm=2" });
    await waitFor(() => expect(requested("llm")).toEqual(["Anthropic"]));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));
    expect(tab("LLM providers")).toHaveTextContent("1");
    expect(options()).toEqual(["Add OpenAI", "Remove Anthropic"]);

    fireEvent.change(screen.getByRole("textbox", { name: /App Name/ }), { target: { value: "Support bot" } });
    fireEvent.change(screen.getByRole("textbox", { name: /Description/ }), { target: { value: "Helps" } });
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));
    addFromPicker("OpenAI");
    openTab("Vector DBs");
    addFromPicker("Pinecone");
    expect(requested("llm")).toEqual(["Anthropic", "OpenAI"]);
    expect(requested("plugin:3:vector-db")).toEqual(["Pinecone"]);
    expect(screen.getByText("Access requested (3)")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Create App" }));

    await waitFor(() =>
      expect(pubClient.post).toHaveBeenCalledWith("/common/apps", {
        name: "Support bot",
        description: "Helps",
        data_source_ids: [],
        llm_ids: [2, 1],
        tool_ids: [],
        plugin_resources: [{ plugin_id: 3, resource_type_slug: "vector-db", instance_ids: ["i1"] }],
      }),
    );
    expect(await screen.findByText("App Submitted")).toBeInTheDocument();
    // markSaved() ran before the success screen replaced the form.
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });

  it("summarises the submitted app instead of repeating the helper text", async () => {
    renderBuilder({ path: "/portal/apps/new?mcp_server=12" });
    await waitFor(() => expect(requested("mcp_server")).toEqual(["Weather MCP"]));
    expect(tab("MCP servers")).toHaveAttribute("aria-selected", "true");
    openTab("Tools");
    addFromPicker("Weather");
    fireEvent.change(screen.getByRole("textbox", { name: /App Name/ }), { target: { value: "Forecaster" } });
    fireEvent.change(screen.getByRole("textbox", { name: /Description/ }), { target: { value: "Daily weather" } });
    fireEvent.click(screen.getByRole("button", { name: "Create App" }));

    const summary = await screen.findByTestId("app-submitted-summary");
    expect(pubClient.post).toHaveBeenCalledWith(
      "/common/apps",
      expect.objectContaining({ tool_ids: [4], mcp_server_ids: [12] }),
    );
    expect(within(summary).getByText("Forecaster")).toBeInTheDocument();
    expect(within(summary).getByText("Daily weather")).toBeInTheDocument();
    expect(requested("tool")).toEqual(["Weather"]);
    expect(requested("mcp_server")).toEqual(["Weather MCP"]);
    // Read-only: nothing to remove on the summary.
    expect(within(summary).queryByRole("button", { name: /Remove/ })).not.toBeInTheDocument();
    expect(within(summary).getByText(/request a Tyk access key/)).toBeInTheDocument();
    expect(screen.queryByText(/You must select at least one resource/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open app" }));
    expect(mockNavigate).toHaveBeenCalledWith("/portal/apps/77");
  });

  it("removes a requested item from the details column and from the picker", async () => {
    renderBuilder({ path: "/portal/apps/new?tool=4" });
    await waitFor(() => expect(requested("tool")).toEqual(["Weather"]));

    fireEvent.click(within(screen.getByTestId("requested-access")).getByRole("button", { name: "Remove Weather" }));
    expect(requested("tool")).toEqual([]);
    expect(screen.getByTestId("requested-access-empty")).toBeInTheDocument();

    fireEvent.click(within(screen.getByTestId("app-access-options")).getByRole("button", { name: "Add Weather" }));
    fireEvent.click(within(screen.getByTestId("app-access-options")).getByRole("button", { name: "Remove Weather" }));
    expect(requested("tool")).toEqual([]);
  });

  // Attaching an asset the plugin gates itself grants nothing, so the builder
  // does not offer it; a type with nothing app-granted has no tab at all.
  it("offers only instances an app credential unlocks and ignores deep links to the rest", async () => {
    renderBuilder({ path: "/portal/apps/new?plugin_resource=9%3Aagent%3Aa1" });
    await waitFor(() => expect(picker()).toBeTruthy());

    expect(tab("Prompts")).toBeUndefined();
    expect(requested("plugin:9:agent")).toEqual([]);
    openTab("Agents");
    expect(options()).toEqual(["Add Proxied Agent"]);
    addFromPicker("Proxied Agent");
    expect(requested("plugin:9:agent")).toEqual(["Proxied Agent"]);
  });

  it("offers only MCP servers AI Studio brokers and ignores deep links to the rest", async () => {
    renderBuilder({ path: "/portal/apps/new?mcp_server=13" });
    await waitFor(() => expect(picker()).toBeTruthy());
    expect(requested("mcp_server")).toEqual([]);
    openTab("MCP servers");
    expect(options()).toEqual(["Add Weather MCP"]);
    expect(screen.getByText(/Served by a Tyk Gateway/)).toBeInTheDocument();
  });

  it("preselects an app-granted instance from a deep link", async () => {
    renderBuilder({ path: "/portal/apps/new?plugin_resource=9%3Aagent%3Aa2" });
    await waitFor(() => expect(requested("plugin:9:agent")).toEqual(["Proxied Agent"]));
    expect(tab("Agents")).toHaveAttribute("aria-selected", "true");
  });

  it("has a Cancel that returns to the apps list when clean and prompts when dirty", async () => {
    renderBuilder();
    await waitFor(() => expect(picker()).toBeTruthy());
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));

    const cancel = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancel);
    expect(mockNavigate).toHaveBeenCalledWith("/portal/apps");

    openTab("Tools");
    addFromPicker("Weather");
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));

    mockNavigate.mockClear();
    fireEvent.click(cancel);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
