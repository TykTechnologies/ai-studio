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

let mockConfig = { apiUrl: "http://localhost", proxyUrl: "http://localhost:9090" };
jest.mock("../../config", () => ({
  getConfig: () => mockConfig,
}));

const pubClient = require("../../admin/utils/pubClient").default;

// The component uses `response.data` directly as the app object, and the
// accessible-* endpoints as plain arrays.
const appFixture = (credentialActive, llmIds = []) => ({
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
      llm_ids: llmIds,
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

const llmFixture = {
  id: "7",
  type: "llms",
  attributes: {
    name: "Acme OpenAI",
    vendor: "openai",
    short_description: "desc",
    default_model: "gpt-4o",
    allowed_models: ["gpt-4o", "gpt-4o-mini"],
  },
};

describe("AppDetailView credential state", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockConfig = { apiUrl: "http://localhost", proxyUrl: "http://localhost:9090" };
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

// The Main Ingress is the gateway's single OpenAI-compatible endpoint for the
// whole app. It differs from the per-LLM endpoints in two ways developers hit as
// 400s -- namespaced model strings and OpenAI-only payloads -- so the portal has
// to state both.
describe("AppDetailView Main Ingress endpoint", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockConfig = {
      apiUrl: "http://localhost",
      proxyURL: "http://gw.example.com",
      unifiedRouterPath: "/v1",
    };
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/apps/1") return Promise.resolve(appFixture(true, [7]));
      if (url === "/common/accessible-llms")
        return Promise.resolve({ data: [llmFixture] });
      return Promise.resolve({ data: [] });
    });
  });

  it("advertises the unified endpoint with its namespaced model name", async () => {
    renderView();

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Main Ingress" })
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText("http://gw.example.com/v1/chat/completions")
    ).toBeInTheDocument();
    // The model prefix is the LLM slug, not the vendor name.
    expect(screen.getByText("acme-openai/gpt-4o")).toBeInTheDocument();
    expect(screen.getByText(/Models must be namespaced/)).toBeInTheDocument();
    expect(screen.getByText(/OpenAI format only/)).toBeInTheDocument();
  });

  it("follows a relocated ingress base path", async () => {
    mockConfig = { ...mockConfig, unifiedRouterPath: "/ai-gw/v1" };

    renderView();

    await waitFor(() => {
      expect(
        screen.getByText("http://gw.example.com/ai-gw/v1/chat/completions")
      ).toBeInTheDocument();
    });
  });

  // An empty path means the gateway does not serve the ingress at all; showing
  // it anyway would send developers to a 404.
  it("hides the section when the ingress is disabled", async () => {
    mockConfig = { ...mockConfig, unifiedRouterPath: "" };

    renderView();

    await waitFor(() => {
      expect(screen.getByText("Per-LLM Endpoints")).toBeInTheDocument();
    });
    expect(
      screen.queryByRole("heading", { name: "Main Ingress" })
    ).not.toBeInTheDocument();
  });

  // The backend is the only source of truth for the path. A missing key must not
  // fall back to "/v1": that would advertise an endpoint on the strength of an
  // assumption about the backend rather than something it actually reported.
  it("hides the section when the backend reports no path at all", async () => {
    mockConfig = { apiUrl: "http://localhost", proxyURL: "http://gw.example.com" };

    renderView();

    await waitFor(() => {
      expect(screen.getByText("Per-LLM Endpoints")).toBeInTheDocument();
    });
    expect(
      screen.queryByRole("heading", { name: "Main Ingress" })
    ).not.toBeInTheDocument();
  });
});
