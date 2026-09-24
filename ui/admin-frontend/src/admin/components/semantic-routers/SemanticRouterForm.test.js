import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import SemanticRouterForm from "./SemanticRouterForm";
import { useEdition } from "../../context/EditionContext";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../common/relationship-picker", () =>
  require("../../../test-utils/component-mocks").relationshipPickerMock,
);

const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const llms = [
  { id: "1", attributes: { name: "OpenAI", vendor: "openai" } },
  { id: "2", attributes: { name: "Anthropic", vendor: "anthropic" } },
];
const modelRouters = [{ id: "4", attributes: { name: "Cheap pool" } }];
const embedders = [
  { id: "3", attributes: { name: "OpenAI small", model: "text-embedding-3-small", vendor: "openai", privacy_score: 60 } },
];
const catalogues = [{ id: "2", attributes: { name: "Platform" } }];

const existingRouter = {
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Smart",
        slug: "smart",
        description: "",
        active: true,
        namespace: "",
        short_description: "",
        long_description: "",
        logo_url: "",
        catalogues: [],
        embedder_id: 3,
        embedder_name: "OpenAI small",
        settings: {
          mode: "shadow",
          allow_explicit_route: true,
          input_scope: "last_user",
          embedding: { llm_id: 1, model: "text-embedding-3-small", timeout_ms: 1500 },
          judge: { enabled: false, model_ref: { llm_id: 0, model: "" }, when: "low_confidence" },
          affinity: { enabled: false },
          default_route: "simple",
        },
        routes: [
          {
            name: "simple",
            description: "Everyday",
            target: { type: "llm", llm_id: 1, model: "gpt-4o-mini" },
          },
          {
            name: "complex",
            description: "Hard reasoning",
            priority: 10,
            keywords: [{ pattern: "prove", regex: false }],
            utterances: ["derive the formula for x"],
            threshold: 0.8,
            target: { type: "model_router", model_router_id: 4, model: "cheap" },
          },
        ],
      },
    },
  },
};

const identity = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: ["semantic-routers:write", "semantic-routers:publish", "embedders:read", "embedders:write"],
  },
};

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity}>
          <SemanticRouterForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

// Opens an MUI Select by its label and clicks an option.
const choose = (label, option, container = document.body) => {
  fireEvent.mouseDown(within(container).getByRole("combobox", { name: label }));
  fireEvent.click(within(screen.getByRole("listbox")).getByText(option));
};

describe("SemanticRouterForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms") return Promise.resolve({ data: { data: llms }, headers: {} });
      if (url === "/model-routers") return Promise.resolve({ data: { data: modelRouters }, headers: {} });
      if (url === "/catalogues") return Promise.resolve({ data: { data: catalogues }, headers: {} });
      if (url === "/embedders") return Promise.resolve({ data: { data: embedders }, headers: {} });
      if (url === "/semantic-routers/5") return Promise.resolve(existingRouter);
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9", attributes: {} } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5", attributes: {} } } });
    apiClient.put.mockResolvedValue({ data: { data: { catalogues: [] } } });
  });

  it("refuses to save without routes and names what is missing", async () => {
    renderForm();
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByText("At least one route is required")).toBeInTheDocument();
    expect(screen.getByText("Name is required")).toBeInTheDocument();
    expect(screen.getByTestId("semantic-router-form-errors")).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("adds, switches the target of, and removes routes", async () => {
    renderForm();
    // Pickers load through listAll, not a bare first page.
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/llms", { params: { page: 1, page_size: 100 } }));
    expect(apiClient.get).toHaveBeenCalledWith("/model-routers", { params: { page: 1, page_size: 100 } });

    fireEvent.click(screen.getByRole("button", { name: "Add Route" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Route" }));
    expect(screen.getByTestId("route-card-1")).toBeInTheDocument();

    const first = screen.getByTestId("route-card-0");
    expect(within(first).getByRole("combobox", { name: "LLM provider" })).toBeInTheDocument();
    fireEvent.click(within(first).getByRole("button", { name: "Model Router" }));
    expect(within(first).getByRole("combobox", { name: "Model router" })).toBeInTheDocument();
    expect(within(first).getByLabelText(/Model alias/)).toBeInTheDocument();
    expect(within(first).queryByRole("combobox", { name: "LLM provider" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove route 2" }));
    expect(screen.queryByTestId("route-card-1")).not.toBeInTheDocument();
  });

  it("flags bad route names, a missing target and examples without an embedder", async () => {
    renderForm();
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/llms", expect.anything()));
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Smart" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Route" }));
    const card = screen.getByTestId("route-card-0");
    fireEvent.change(within(card).getByLabelText(/^Route name/), { target: { value: "auto" } });
    fireEvent.change(screen.getByLabelText("Example prompts for route 1"), { target: { value: "hello there" } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    expect(await within(card).findByText('"auto" is reserved')).toBeInTheDocument();
    expect(within(card).getByText("Pick an LLM provider")).toBeInTheDocument();
    expect(within(card).getByText("Model is required")).toBeInTheDocument();
    expect(screen.getByText("Routes with examples need an embedder")).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("picks an embedder and submits the full config", async () => {
    renderForm();
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/llms", expect.anything()));
    await screen.findAllByTestId("relationship-picker");

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Smart" } });

    // Route 1: simple, on an LLM
    fireEvent.click(screen.getByRole("button", { name: "Add Route" }));
    const simple = screen.getByTestId("route-card-0");
    fireEvent.change(within(simple).getByLabelText(/^Route name/), { target: { value: "simple" } });
    await waitFor(() => expect(within(simple).getByRole("combobox", { name: "LLM provider" })).toBeInTheDocument());
    choose("LLM provider", "OpenAI (openai)", simple);
    fireEvent.change(within(simple).getByLabelText(/^Model/), { target: { value: "gpt-4o-mini" } });

    // Route 2: complex, through a model router, with a regex keyword and examples
    fireEvent.click(screen.getByRole("button", { name: "Add Route" }));
    const complex = screen.getByTestId("route-card-1");
    fireEvent.change(within(complex).getByLabelText(/^Route name/), { target: { value: "complex" } });
    fireEvent.click(within(complex).getByRole("button", { name: "Model Router" }));
    choose("Model router", "Cheap pool", complex);
    fireEvent.change(within(complex).getByLabelText(/Model alias/), { target: { value: "cheap" } });
    const keyword = screen.getByLabelText("Keyword for route 2");
    fireEvent.change(keyword, { target: { value: "prove" } });
    fireEvent.keyDown(keyword, { key: "Enter" });
    fireEvent.click(within(screen.getByTestId("route-1-keywords")).getByRole("checkbox"));
    fireEvent.change(screen.getByLabelText("Example prompts for route 2"), {
      target: { value: "derive the formula\n\n  prove that x  " },
    });
    fireEvent.change(within(complex).getByLabelText(/^Threshold/), { target: { value: "0.8" } });

    choose("Default route", "simple");

    // The embedding stage uses an embedder (picked, or created inline).
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/embedders", expect.anything()));
    expect(screen.getByRole("button", { name: "New embedder" })).toBeInTheDocument();
    choose("Embedder", "OpenAI small");

    const catalogPicker = screen.getAllByTestId("relationship-picker").find((el) => el.dataset.itemLabel === "catalog");
    fireEvent.click(within(catalogPicker).getByTestId("relationship-picker-add"));

    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    const [url, body] = apiClient.post.mock.calls[0];
    expect(url).toBe("/semantic-routers");
    expect(body.data.type).toBe("semantic-routers");
    const attrs = body.data.attributes;
    expect(attrs).toEqual(expect.objectContaining({ name: "Smart", slug: "smart", active: false }));
    expect(attrs.settings).toEqual({
      mode: "enforce",
      allow_explicit_route: false,
      input_scope: "last_user",
      judge: { enabled: false, model_ref: { llm_id: 0, model: "" }, when: "low_confidence" },
      affinity: { enabled: false, header: "X-Tyk-Session-Id" },
      default_route: "simple",
    });
    expect(attrs.embedder_id).toBe(3);
    expect(attrs.routes).toEqual([
      {
        name: "simple",
        description: "",
        priority: 0,
        keywords: [],
        utterances: [],
        target: { type: "llm", llm_id: 1, model: "gpt-4o-mini" },
      },
      {
        name: "complex",
        description: "",
        priority: 0,
        keywords: [{ pattern: "prove", regex: true }],
        utterances: ["derive the formula", "prove that x"],
        threshold: 0.8,
        target: { type: "model_router", model_router_id: 4, model: "cheap" },
      },
    ]);
    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/semantic-routers/9/catalogues", { catalogue_ids: [2] }));
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/semantic-routers", {
        state: { snackbar: { message: "Semantic Router created successfully", severity: "success" } },
      }),
    );
  });

  it("loads an existing router and PATCHes it back unchanged", async () => {
    mockParams = { id: "5" };
    renderForm();
    await screen.findByDisplayValue("complex");
    expect(screen.getByDisplayValue("cheap")).toBeInTheDocument();
    expect(screen.getByDisplayValue("derive the formula for x")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [url, body] = apiClient.patch.mock.calls[0];
    expect(url).toBe("/semantic-routers/5");
    const attrs = body.data.attributes;
    expect(attrs.settings.mode).toBe("shadow");
    expect(attrs.settings.allow_explicit_route).toBe(true);
    // The embedder goes by id; settings keep only its timeout.
    expect(attrs.embedder_id).toBe(3);
    expect(attrs.settings.embedding).toEqual({ timeout_ms: 1500 });
    expect(attrs.routes[1]).toEqual({
      name: "complex",
      description: "Hard reasoning",
      priority: 10,
      keywords: [{ pattern: "prove", regex: false }],
      utterances: ["derive the formula for x"],
      threshold: 0.8,
      target: { type: "model_router", model_router_id: 4, model: "cheap" },
    });
    expect(apiClient.put).not.toHaveBeenCalled();
  });

  it("shows the server's validation detail when the save is refused", async () => {
    mockParams = { id: "5" };
    apiClient.patch.mockRejectedValue({
      response: {
        status: 400,
        data: { errors: [{ title: "Bad Request", detail: "invalid semantic router: settings.embedding: X (anthropic) does not provide embeddings" }] },
      },
    });
    renderForm();
    await screen.findByDisplayValue("complex");
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    expect(await screen.findByTestId("semantic-router-form-errors")).toHaveTextContent("does not provide embeddings");
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("tests the unsaved draft from the editor", async () => {
    mockParams = { id: "5" };
    apiClient.post.mockResolvedValue({
      data: { data: { decision: { route: "complex", reason: "keyword", score: 1, latency_ms: 2, trace: [] }, target: null } },
    });
    renderForm();
    await screen.findByDisplayValue("complex");
    fireEvent.change(screen.getByLabelText("Test prompt"), { target: { value: "prove it" } });
    fireEvent.click(screen.getByRole("button", { name: "Test routing" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    const [url, body] = apiClient.post.mock.calls[0];
    expect(url).toBe("/semantic-routers/test");
    expect(body.messages).toEqual([{ role: "user", content: "prove it" }]);
    expect(body.router.slug).toBe("smart");
    expect(body.router.routes.map((r) => r.name)).toEqual(["simple", "complex"]);
    expect(await screen.findByTestId("decision-route")).toHaveTextContent("complex");
  });
});
