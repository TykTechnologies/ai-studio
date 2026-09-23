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
  type: "model_router",
  id: "5",
  attributes: {
    name: "Production router",
    short_description: "Routes GPT traffic",
    kind: "openai",
    kind_label: "Model Router",
    privacy_score: 40,
    catalogs: [{ id: "1", name: "Default" }],
    access_granted_via_app: true,
    router_slug: "prod",
    router_models: ["prod/gpt-4o", "prod/gpt-4o-mini"],
    router_llms: [
      { id: 1, name: "OpenAI EU", vendor: "openai" },
      { id: 2, name: "Azure backup", vendor: "azure" },
    ],
  },
};

const apps = {
  data: [
    { id: "1", attributes: { name: "Router app", llm_ids: [], model_router_ids: [5], is_active: true, credential_active: true } },
    { id: "2", attributes: { name: "LLM app", llm_ids: [1], model_router_ids: [], is_active: true, credential_active: true } },
  ],
};

const renderDetail = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/portal/catalog/model-routers/5"]}>
        <Routes>
          <Route path="/portal/catalog/model-routers/:id" element={<AssetDetail type="model_router" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("AssetDetail (model router)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockConfig = { proxyURL: "http://gw.example.com", unifiedRouterPath: "/v1" };
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/catalog/model-routers/5") return Promise.resolve({ data: { data: router } });
      if (url === "/common/apps") return Promise.resolve({ data: apps });
      return Promise.reject(new Error("unexpected " + url));
    });
  });

  it("lists the model strings to send, the endpoint and the LLMs it routes to", async () => {
    renderDetail();
    expect(await screen.findByText("Production router")).toBeInTheDocument();
    expect(screen.getByText("Model Router")).toBeInTheDocument();

    const models = screen.getByTestId("router-models");
    expect(models).toHaveTextContent("prod/gpt-4o");
    expect(models).toHaveTextContent("prod/gpt-4o-mini");
    expect(screen.getByRole("button", { name: "Copy model prod/gpt-4o" })).toBeInTheDocument();
    expect(screen.getByTestId("router-endpoint")).toHaveTextContent("http://gw.example.com/v1/chat/completions");
    expect(screen.getByTestId("router-example")).toHaveTextContent('"model": "prod/gpt-4o"');

    const llms = screen.getByTestId("router-llms");
    expect(llms).toHaveTextContent("OpenAI EU");
    expect(llms).toHaveTextContent("Azure backup");

    // Only the app granted the router is listed as using it.
    expect(screen.getByTestId("asset-apps")).toHaveTextContent("Router app");
    expect(screen.getByTestId("asset-apps")).not.toHaveTextContent("LLM app");
  });

  it("builds an app with the router preselected", async () => {
    renderDetail();
    fireEvent.click(await screen.findByTestId("asset-build-app"));
    expect(mockNavigate).toHaveBeenCalledWith("/portal/app/new?model_router=5");
  });

  it("names the unified endpoint path when the gateway address is not known", async () => {
    mockConfig = {};
    renderDetail();
    expect(await screen.findByText("Production router")).toBeInTheDocument();
    expect(screen.queryByTestId("router-endpoint")).not.toBeInTheDocument();
    expect(screen.getByTestId("router-call-section")).toHaveTextContent("POST /v1/chat/completions");
  });
});
