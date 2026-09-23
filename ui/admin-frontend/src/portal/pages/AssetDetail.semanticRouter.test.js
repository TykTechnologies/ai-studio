import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../admin/utils/testTheme";
import AssetDetail from "./AssetDetail";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
let mockConfig = {};
jest.mock("../../config", () => ({ getConfig: () => mockConfig }));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const pubClient = require("../../admin/utils/pubClient").default;

const router = {
  type: "semantic_router",
  id: "7",
  attributes: {
    name: "Smart router",
    short_description: "Sends hard prompts to a bigger model",
    kind: "openai",
    kind_label: "Semantic Router",
    privacy_score: 40,
    catalogs: [{ id: "1", name: "Default" }],
    access_granted_via_app: true,
    router_slug: "smart",
    router_models: ["smart/auto", "smart/complex"],
    router_routes: [
      { name: "simple", description: "Everyday questions", default: true },
      { name: "complex", description: "Hard reasoning" },
    ],
    router_llms: [{ id: 1, name: "OpenAI EU", vendor: "openai" }],
  },
};

const apps = {
  data: [
    { id: "1", attributes: { name: "Smart app", llm_ids: [], semantic_router_ids: [7], is_active: true, credential_active: true } },
    { id: "2", attributes: { name: "LLM app", llm_ids: [1], semantic_router_ids: [], is_active: true, credential_active: true } },
  ],
};

const renderDetail = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/portal/catalog/semantic-routers/7"]}>
        <Routes>
          <Route path="/portal/catalog/semantic-routers/:id" element={<AssetDetail type="semantic_router" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("AssetDetail (semantic router)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockConfig = { proxyURL: "http://gw.example.com", unifiedRouterPath: "/v1" };
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/catalog/semantic-routers/7") return Promise.resolve({ data: { data: router } });
      if (url === "/common/apps") return Promise.resolve({ data: apps });
      return Promise.reject(new Error("unexpected " + url));
    });
  });

  it("lists the model strings, the routes with the default marked, and the LLMs", async () => {
    renderDetail();
    expect(await screen.findByText("Smart router")).toBeInTheDocument();
    expect(screen.getByText("Semantic Router")).toBeInTheDocument();

    const models = screen.getByTestId("router-models");
    expect(models).toHaveTextContent("smart/auto");
    expect(models).toHaveTextContent("smart/complex");
    expect(screen.getByTestId("router-example")).toHaveTextContent('"model": "smart/auto"');

    const routes = screen.getByTestId("router-routes");
    expect(routes).toHaveTextContent("simple");
    expect(routes).toHaveTextContent("Everyday questions");
    expect(routes).toHaveTextContent("Default");
    expect(routes).toHaveTextContent("Hard reasoning");

    expect(screen.getByTestId("router-llms")).toHaveTextContent("OpenAI EU");
    expect(screen.getByTestId("asset-apps")).toHaveTextContent("Smart app");
    expect(screen.getByTestId("asset-apps")).not.toHaveTextContent("LLM app");
  });

  it("builds an app with the router preselected", async () => {
    renderDetail();
    fireEvent.click(await screen.findByTestId("asset-build-app"));
    expect(mockNavigate).toHaveBeenCalledWith("/portal/app/new?semantic_router=7");
  });
});
