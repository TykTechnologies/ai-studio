import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import SemanticRouterTestPanel, { describeTarget } from "./SemanticRouterTestPanel";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { post: jest.fn() },
}));

const llms = [{ id: "2", attributes: { name: "Anthropic" } }];
const modelRouters = [{ id: "4", attributes: { name: "Cheap pool" } }];

const decision = {
  route: "complex",
  reason: "embedding",
  score: 0.87,
  shadow_route: "",
  input: "prove that the sum converges",
  latency_ms: 42,
  trace: [
    { stage: "keywords", scores: {}, detail: "no keyword matched", latency_ms: 0 },
    { stage: "embeddings", route: "complex", scores: { complex: 0.87, simple: 0.41 }, latency_ms: 38 },
    { stage: "judge", skipped: true, detail: "confident", latency_ms: 0 },
  ],
};

const renderPanel = (props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <SemanticRouterTestPanel llms={llms} modelRouters={modelRouters} routeNames={["simple", "complex"]} {...props} />
    </ThemeProvider>,
  );

const typeAndRun = (text) => {
  fireEvent.change(screen.getByLabelText("Test prompt"), { target: { value: text } });
  fireEvent.click(screen.getByRole("button", { name: "Test routing" }));
};

describe("SemanticRouterTestPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.post.mockResolvedValue({
      data: { data: { decision, target: { type: "llm", llm_id: 2, model: "claude-opus-4" } } },
    });
  });

  it("tests a saved router and renders the decision and per-stage trace", async () => {
    renderPanel({ routerId: "5" });
    expect(screen.getByRole("button", { name: "Test routing" })).toBeDisabled();
    typeAndRun("prove that the sum converges");

    await screen.findByTestId("semantic-router-decision");
    expect(apiClient.post).toHaveBeenCalledWith("/semantic-routers/5/test", {
      messages: [{ role: "user", content: "prove that the sum converges" }],
    });
    expect(screen.getByTestId("decision-route")).toHaveTextContent("complex");
    expect(screen.getByTestId("decision-reason")).toHaveTextContent("Embedding similarity");
    expect(screen.getByTestId("decision-score")).toHaveTextContent("0.87");
    expect(screen.getByTestId("decision-target")).toHaveTextContent("claude-opus-4 on Anthropic");
    expect(screen.queryByTestId("decision-shadow-route")).not.toBeInTheDocument();
    expect(screen.getByText("42 ms")).toBeInTheDocument();

    const rows = screen.getAllByTestId("trace-row");
    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent("embeddings");
    const scores = within(rows[1]).getAllByTestId("stage-score");
    // Highest score first.
    expect(scores[0]).toHaveTextContent("complex");
    expect(scores[0]).toHaveTextContent("0.87");
    expect(scores[1]).toHaveTextContent("simple");
    expect(within(rows[2]).getByText("skipped")).toBeInTheDocument();
  });

  it("shows the shadow route when the router only records its choice", async () => {
    apiClient.post.mockResolvedValue({
      data: { data: { decision: { ...decision, route: "simple", reason: "default", shadow_route: "complex" }, target: null } },
    });
    renderPanel({ routerId: "5" });
    typeAndRun("prove it");
    expect(await screen.findByTestId("decision-shadow-route")).toHaveTextContent("complex");
    expect(screen.getByTestId("decision-route")).toHaveTextContent("simple");
  });

  it("tests a draft and can name a route explicitly when explicit routes are on", async () => {
    const draft = { name: "Smart", slug: "smart", settings: {}, routes: [] };
    renderPanel({ getDraft: () => draft, allowExplicit: true });
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Model" }));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("simple"));
    typeAndRun("hello");

    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(apiClient.post).toHaveBeenCalledWith("/semantic-routers/test", {
      router: draft,
      messages: [{ role: "user", content: "hello" }],
      model: "simple",
    });
  });

  it("shows the server's error detail", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 400, data: { errors: [{ title: "Bad Request", detail: "routes[0].target.model: required" }] } },
    });
    renderPanel({ routerId: "5" });
    typeAndRun("hello");
    expect(await screen.findByTestId("semantic-router-test-error")).toHaveTextContent("routes[0].target.model: required");
  });

  it("describes model-router targets by the alias and router name", () => {
    expect(describeTarget({ type: "model_router", model_router_id: 4, model: "cheap" }, { modelRouters })).toBe(
      "cheap via model router Cheap pool",
    );
    expect(describeTarget({ type: "llm", llm_id: 9, model: "gpt-4o" }, { llms })).toBe("gpt-4o on LLM #9");
  });
});
