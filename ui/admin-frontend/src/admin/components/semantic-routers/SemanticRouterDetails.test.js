import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import SemanticRouterDetails from "./SemanticRouterDetails";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), patch: jest.fn(), post: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../common/UsedBySection", () => ({
  __esModule: true,
  default: () => null,
}));

const router = {
  id: "5",
  attributes: {
    name: "Smart",
    slug: "smart",
    active: true,
    namespace: "",
    models: ["smart/auto", "smart/simple", "smart/complex"],
    catalogues: [{ id: 4, name: "Platform" }],
    settings: {
      mode: "shadow",
      allow_explicit_route: true,
      input_scope: "all_user",
      embedding: { llm_id: 1, model: "text-embedding-3-small" },
      judge: { enabled: false, model_ref: { llm_id: 0, model: "" } },
      affinity: { enabled: false },
      default_route: "simple",
    },
    routes: [
      { name: "simple", description: "Everyday", target: { type: "llm", llm_id: 1, model: "gpt-4o-mini" } },
      {
        name: "complex",
        description: "Hard reasoning",
        keywords: [{ pattern: "prove" }, { pattern: "derive" }],
        utterances: ["a", "b", "c"],
        target: { type: "model_router", model_router_id: 4, model: "cheap" },
      },
    ],
  },
};

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/semantic-routers/5"]}>
        <PermissionsProvider identity={identity(permissions)}>
          <Routes>
            <Route path="/admin/semantic-routers/:id" element={<SemanticRouterDetails />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("SemanticRouterDetails", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockImplementation((url) => {
      if (url === "/semantic-routers/5") return Promise.resolve({ data: { data: router } });
      if (url === "/llms") return Promise.resolve({ data: { data: [{ id: "1", attributes: { name: "OpenAI" } }] }, headers: {} });
      if (url === "/model-routers") return Promise.resolve({ data: { data: [{ id: "4", attributes: { name: "Cheap pool" } }] }, headers: {} });
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it("shows the model strings, routes with their targets, settings and catalogs", async () => {
    renderWith(["semantic-routers:read"]);
    expect(await screen.findByTestId("router-model-strings")).toHaveTextContent("smart/auto");
    expect(screen.getByTestId("router-model-strings")).toHaveTextContent("smart/complex");
    expect(screen.getByRole("button", { name: "Copy smart/auto" })).toBeInTheDocument();

    const rows = await screen.findAllByTestId("route-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Default");
    expect(await screen.findByText("cheap via model router Cheap pool")).toBeInTheDocument();
    expect(rows[1]).toHaveTextContent("Hard reasoning");

    expect(screen.getByTestId("router-settings")).toHaveTextContent("Shadow");
    expect(screen.getByTestId("router-settings")).toHaveTextContent("All user messages");
    expect(screen.getByTestId("router-catalogues")).toHaveTextContent("Platform");
    // Read-only: no toggle, no edit, no test panel.
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("semantic-router-test-panel")).not.toBeInTheDocument();
  });

  it("publish + write: toggle, edit and the test panel are shown", async () => {
    renderWith(["semantic-routers:write", "semantic-routers:publish"]);
    expect(await screen.findByRole("button", { name: "Deactivate" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(screen.getByTestId("semantic-router-test-panel")).toBeInTheDocument();
  });
});
