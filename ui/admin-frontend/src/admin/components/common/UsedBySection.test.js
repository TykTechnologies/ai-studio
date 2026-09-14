import React from "react";
import { render, screen, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import UsedBySection, { CatalogueTeamsSection } from "./UsedBySection";
import apiClient from "../../utils/apiClient";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const renderWith = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>,
  );

const dependents = (attributes) => ({ data: { data: { attributes } } });
const empty = { apps: [], catalogues: [], llms: [], tools: [], datasources: [], agents: [], model_routers: [], chats: [], total: 0 };

describe("UsedBySection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows a loading state while the dependents load", () => {
    apiClient.get.mockReturnValue(new Promise(() => {}));
    renderWith(<UsedBySection resourcePath="llms" id="3" objectLabel="LLM provider" />);
    expect(screen.getByText("Used by")).toBeInTheDocument();
    expect(screen.getByTestId("used-by-loading")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/llms/3/dependents");
  });

  it("renders one row per non-empty group with links to the admin pages", async () => {
    apiClient.get.mockResolvedValue(
      dependents({
        ...empty,
        apps: [{ id: 1, name: "Billing Copilot" }, { id: 4, name: "Quickstart" }],
        catalogues: [{ id: 2, name: "Platform" }],
        chats: [{ id: 9, name: "Support room" }],
        total: 4,
      }),
    );
    renderWith(<UsedBySection resourcePath="llms" id="3" objectLabel="LLM provider" />);
    const groups = await screen.findByTestId("used-by-groups");
    expect(within(groups).getByText("Apps (2):")).toBeInTheDocument();
    expect(within(groups).getByText("Catalogs (1):")).toBeInTheDocument();
    expect(within(groups).getByText("Chats (1):")).toBeInTheDocument();
    expect(screen.queryByText(/Tools \(/)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Billing Copilot" })).toHaveAttribute("href", "/admin/apps/1");
    expect(screen.getByRole("link", { name: "Quickstart" })).toHaveAttribute("href", "/admin/apps/4");
    expect(screen.getByRole("link", { name: "Platform" })).toHaveAttribute("href", "/admin/catalogs/llms/2");
    expect(screen.getByRole("link", { name: "Support room" })).toHaveAttribute("href", "/admin/chats/9");
  });

  it("links a tool's catalogues to the tool catalogue pages", async () => {
    apiClient.get.mockResolvedValue(dependents({ ...empty, catalogues: [{ id: 5, name: "Ops tools" }], total: 1 }));
    renderWith(<UsedBySection resourcePath="tools" id="8" objectLabel="tool" />);
    expect(await screen.findByRole("link", { name: "Ops tools" })).toHaveAttribute("href", "/admin/catalogs/tools/5");
  });

  it("says nothing uses the object when the total is zero", async () => {
    apiClient.get.mockResolvedValue(dependents(empty));
    renderWith(<UsedBySection resourcePath="secrets" id="2" objectLabel="secret" />);
    expect(await screen.findByTestId("used-by-empty")).toHaveTextContent("Nothing uses this secret yet.");
  });

  it("fails quietly when the endpoint errors", async () => {
    apiClient.get.mockRejectedValue(new Error("boom"));
    jest.spyOn(console, "error").mockImplementation(() => {});
    renderWith(<UsedBySection resourcePath="filters" id="2" objectLabel="filter" />);
    expect(await screen.findByTestId("used-by-error")).toHaveTextContent("Could not load usage.");
    console.error.mockRestore();
  });
});

describe("CatalogueTeamsSection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists the teams with member counts and links to the team page", async () => {
    apiClient.get.mockResolvedValue({
      data: { data: [{ id: 2, name: "Platform", member_count: 1 }, { id: 3, name: "Data", member_count: 4 }] },
    });
    renderWith(<CatalogueTeamsSection resourcePath="data-catalogues" id="6" />);
    expect(await screen.findByText("Teams (2):")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/data-catalogues/6/groups");
    expect(screen.getByRole("link", { name: "Platform" })).toHaveAttribute("href", "/admin/groups/2");
    expect(screen.getByText("(1 member)")).toBeInTheDocument();
    expect(screen.getByText("(4 members)")).toBeInTheDocument();
  });

  it("says no teams use the catalog when the list is empty", async () => {
    apiClient.get.mockResolvedValue({ data: { data: [] } });
    renderWith(<CatalogueTeamsSection resourcePath="catalogues" id="6" />);
    expect(await screen.findByTestId("used-by-empty")).toHaveTextContent("No teams use this catalog yet.");
  });

  it("fails quietly when the endpoint is missing", async () => {
    apiClient.get.mockRejectedValue({ response: { status: 404 } });
    jest.spyOn(console, "error").mockImplementation(() => {});
    renderWith(<CatalogueTeamsSection resourcePath="tool-catalogues" id="6" />);
    expect(await screen.findByTestId("used-by-error")).toBeInTheDocument();
    console.error.mockRestore();
  });
});
