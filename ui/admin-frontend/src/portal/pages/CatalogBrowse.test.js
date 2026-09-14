import React from "react";
import { render, screen, waitFor, within, fireEvent, act } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import testTheme from "../../admin/utils/testTheme";
import CatalogBrowse from "./CatalogBrowse";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../../config", () => ({ getConfig: () => ({}) }));

const pubClient = require("../../admin/utils/pubClient").default;

const item = (type, id, attributes) => ({ type, id: String(id), attributes });

const all = [
  item("llm", 2, { name: "Bedrock Claude", short_description: "Long context", kind: "bedrock", privacy_score: 80, created_at: "2026-09-10T00:00:00Z", catalogs: [{ id: "2", name: "Platform" }] }),
  item("datasource", 3, { name: "Docs index", short_description: "Product docs", kind: "pgvector", privacy_score: 20, created_at: "2026-09-05T00:00:00Z", catalogs: [{ id: "1", name: "Default data" }] }),
  item("llm", 1, { name: "Acme OpenAI", short_description: "Fast general model", kind: "openai", privacy_score: 40, created_at: "2026-09-01T00:00:00Z", catalogs: [{ id: "1", name: "Default" }] }),
  item("tool", 4, { name: "Weather API", short_description: "Forecasts", kind: "rest", privacy_score: 10, created_at: "2026-08-01T00:00:00Z", operations: ["getForecast"], catalogs: [] }),
];

const facets = {
  counts: { llm: 2, datasource: 1, tool: 1, plugin_resource: 0 },
  kinds: [
    { type: "llm", kind: "bedrock", label: "AWS Bedrock", count: 1 },
    { type: "llm", kind: "openai", label: "OpenAI", count: 1 },
    { type: "datasource", kind: "pgvector", count: 1 },
    { type: "tool", kind: "rest", count: 1 },
  ],
  catalogs: [
    { type: "llm", id: "1", name: "Default" },
    { type: "llm", id: "2", name: "Platform" },
    { type: "datasource", id: "1", name: "Default data" },
  ],
  resource_types: [],
};

/** A page response the way the server shapes it. */
const page = (data, { total = data.length, page: p = 1, page_size = 25 } = {}) => ({
  data,
  meta: { ...facets, total, page: p, page_size, total_pages: Math.max(1, Math.ceil(total / page_size)) },
});

/** The server: a tiny stand-in that honours the parameters it is sent. */
const serverFor = (items = all) => (url, options = {}) => {
  const params = options.params || {};
  let rows = items;
  if (params.type) rows = rows.filter((r) => r.type === params.type);
  if (params.kind) rows = rows.filter((r) => r.attributes.kind === params.kind);
  if (params.catalog) {
    const [type, id] = params.catalog.split(":");
    rows = rows.filter((r) => r.type === type && r.attributes.catalogs.some((c) => c.id === id));
  }
  if (params.q) rows = rows.filter((r) => `${r.attributes.name} ${r.attributes.short_description}`.toLowerCase().includes(params.q.toLowerCase()));
  const size = Number(params.page_size) || 25;
  const current = Number(params.page) || 1;
  const slice = rows.slice((current - 1) * size, current * size);
  return Promise.resolve({ data: page(slice, { total: rows.length, page: current, page_size: size }) });
};

const LocationProbe = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
};

const renderBrowse = (path = "/portal/catalog") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/portal/catalog" element={<CatalogBrowse />} />
          <Route path="/portal/catalog/llms" element={<CatalogBrowse type="llm" />} />
          <Route path="/portal/catalog/tools" element={<CatalogBrowse type="tool" />} />
          <Route path="/portal/catalog/llms/:id" element={<div>llm detail</div>} />
          <Route path="/portal/app/new" element={<div>app builder</div>} />
        </Routes>
        <LocationProbe />
      </MemoryRouter>
    </ThemeProvider>
  );

const cardNames = () =>
  screen.getAllByTestId("asset-card").map((card) => within(card).getByRole("heading", { level: 3 }).textContent);

const lastParams = () => pubClient.get.mock.calls[pubClient.get.mock.calls.length - 1][1].params;

// Users do not care which catalog holds an asset: one searchable, filterable
// grid of everything they can use, newest first (UX review D4). The server
// does the searching, filtering, sorting and paging; the page sends the
// controls' state as parameters and renders the page it gets back.
describe("CatalogBrowse", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockImplementation(serverFor());
  });

  it("asks for the first page with no filters and renders it with type chips", async () => {
    renderBrowse();
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(4));
    expect(lastParams()).toEqual({});
    expect(cardNames()).toEqual(["Bedrock Claude", "Docs index", "Acme OpenAI", "Weather API"]);
    expect(screen.getByTestId("catalog-count")).toHaveTextContent("4 assets");
    expect(screen.getAllByTestId("asset-type-chip")).toHaveLength(4);
    expect(screen.getByRole("tab", { name: "All (4)" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByRole("navigation", { name: /pagination/i })).not.toBeInTheDocument();
  });

  it("sends the search term from the query string to the server", async () => {
    renderBrowse("/portal/catalog?q=forecasts");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(1));
    expect(lastParams()).toEqual({ q: "forecasts" });
    expect(cardNames()).toEqual(["Weather API"]);
  });

  it("debounces typing into one request and resets the page", async () => {
    jest.useFakeTimers();
    try {
      renderBrowse("/portal/catalog?page=2");
      await act(async () => {
        jest.advanceTimersByTime(0);
      });
      const input = screen.getByPlaceholderText("Search by name, description, model, operation or vendor");
      fireEvent.change(input, { target: { value: "doc" } });
      fireEvent.change(input, { target: { value: "docs" } });
      await act(async () => {
        jest.advanceTimersByTime(400);
      });
      expect(screen.getByTestId("location")).toHaveTextContent("/portal/catalog?q=docs");
      expect(lastParams()).toEqual({ q: "docs" });
    } finally {
      jest.useRealTimers();
    }
  });

  it("scopes to a type from the route and passes the catalog filter through", async () => {
    renderBrowse("/portal/catalog/llms?catalog=llm:2");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(1));
    expect(lastParams()).toEqual({ type: "llm", catalog: "llm:2" });
    expect(cardNames()).toEqual(["Bedrock Claude"]);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("LLM providers");
    // Type is fixed by the route, so cards do not repeat it, and the tab
    // counts come from the facets, not the page.
    expect(screen.queryAllByTestId("asset-type-chip")).toHaveLength(0);
    expect(screen.getByRole("tab", { name: "LLM providers (2)" })).toHaveAttribute("aria-selected", "true");
  });

  it("pages through results with the shared pagination control", async () => {
    renderBrowse("/portal/catalog?page_size=3");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(3));
    expect(screen.getByTestId("catalog-count")).toHaveTextContent("1–3 of 4 assets");
    fireEvent.click(screen.getByRole("button", { name: "Go to page 2" }));
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(1));
    expect(lastParams()).toEqual({ page: "2", page_size: "3" });
    expect(screen.getByTestId("catalog-count")).toHaveTextContent("4–4 of 4 assets");
    expect(screen.getByTestId("location")).toHaveTextContent("/portal/catalog?page_size=3&page=2");
  });

  it("offers to clear the search when nothing matches", async () => {
    renderBrowse("/portal/catalog?q=zzz");
    await waitFor(() => expect(screen.getByTestId("catalog-no-matches")).toBeInTheDocument());
    expect(screen.getByText("No matches for “zzz”")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear search and filters" }));
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(4));
  });

  it("opens the builder from Build app and the detail page from the card", async () => {
    renderBrowse("/portal/catalog/tools");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(1));
    fireEvent.click(screen.getByTestId("asset-card-build"));
    expect(screen.getByTestId("location")).toHaveTextContent("/portal/app/new?tool=4");
  });

  it("navigates to the LLM detail page when the card is clicked", async () => {
    renderBrowse("/portal/catalog/llms");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(2));
    fireEvent.click(screen.getByRole("link", { name: "Acme OpenAI" }));
    expect(screen.getByText("llm detail")).toBeInTheDocument();
  });

  it("explains an empty catalog", async () => {
    pubClient.get.mockResolvedValue({
      data: { data: [], meta: { total: 0, page: 1, page_size: 25, total_pages: 1, counts: {}, kinds: [], catalogs: [], resource_types: [] } },
    });
    renderBrowse();
    await waitFor(() => expect(screen.getByText("Nothing to build with yet")).toBeInTheDocument());
  });
});
