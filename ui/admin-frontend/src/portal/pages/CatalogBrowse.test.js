import React from "react";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
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

const catalog = {
  data: [
    item("llm", 1, { name: "Acme OpenAI", short_description: "Fast general model", kind: "openai", privacy_score: 40, created_at: "2026-09-01T00:00:00Z", catalogs: [{ id: "1", name: "Default" }] }),
    item("llm", 2, { name: "Bedrock Claude", short_description: "Long context", kind: "bedrock", privacy_score: 80, created_at: "2026-09-10T00:00:00Z", catalogs: [{ id: "2", name: "Platform" }] }),
    item("datasource", 3, { name: "Docs index", short_description: "Product docs", kind: "pgvector", privacy_score: 20, created_at: "2026-09-05T00:00:00Z", catalogs: [{ id: "1", name: "Default data" }] }),
    item("tool", 4, { name: "Weather API", short_description: "Forecasts", kind: "rest", privacy_score: 10, created_at: "2026-08-01T00:00:00Z", operations: ["getForecast"], catalogs: [] }),
  ],
  meta: {
    total: 4,
    counts: { llm: 2, datasource: 1, tool: 1, plugin_resource: 0 },
    catalogs: [
      { type: "llm", id: "1", name: "Default" },
      { type: "llm", id: "2", name: "Platform" },
      { type: "datasource", id: "1", name: "Default data" },
    ],
    resource_types: [],
  },
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

// Users do not care which catalog holds an asset: one searchable, filterable
// grid of everything they can use, newest first (UX review D4).
describe("CatalogBrowse", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({ data: catalog });
  });

  it("lists everything newest first with type chips and a count", async () => {
    renderBrowse();
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(4));
    expect(cardNames()).toEqual(["Bedrock Claude", "Docs index", "Acme OpenAI", "Weather API"]);
    expect(screen.getByTestId("catalog-count")).toHaveTextContent("4 assets");
    expect(screen.getAllByTestId("asset-type-chip")).toHaveLength(4);
    expect(screen.getByRole("tab", { name: "All (4)" })).toHaveAttribute("aria-selected", "true");
  });

  it("searches across names and descriptions from the query string", async () => {
    renderBrowse("/portal/catalog?q=forecasts");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(1));
    expect(cardNames()).toEqual(["Weather API"]);
    expect(screen.getByTestId("catalog-count")).toHaveTextContent("1 of 4 assets");
  });

  it("scopes to a type from the route and filters by catalog from the query", async () => {
    renderBrowse("/portal/catalog/llms?catalog=llm:2");
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(1));
    expect(cardNames()).toEqual(["Bedrock Claude"]);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("LLM providers");
    // Type is fixed by the route, so cards do not repeat it.
    expect(screen.queryAllByTestId("asset-type-chip")).toHaveLength(0);
    expect(screen.getByRole("tab", { name: "LLM providers (2)" })).toHaveAttribute("aria-selected", "true");
  });

  it("offers to clear the search when nothing matches", async () => {
    renderBrowse("/portal/catalog?q=zzz");
    await waitFor(() => expect(screen.getByTestId("catalog-no-matches")).toBeInTheDocument());
    expect(screen.getByText("No matches for “zzz”")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear search and filters" }));
    await waitFor(() => expect(screen.getAllByTestId("asset-card")).toHaveLength(4));
  });

  it("opens the detail page from the card and the builder from Build app", async () => {
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
    pubClient.get.mockResolvedValue({ data: { data: [], meta: { total: 0, counts: {}, catalogs: [], resource_types: [] } } });
    renderBrowse();
    await waitFor(() => expect(screen.getByText("Nothing to build with yet")).toBeInTheDocument());
  });
});
