import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import FilterForm from "./FilterForm";

// 4uc.8: tool-result settings survive a save unchanged, and a response
// filter never sends what Normalize rejects (quarantine) or ignores there.

jest.mock("prismjs/themes/prism-tomorrow.css", () => ({}));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn() },
}));
jest.mock("../common/EdgeAvailabilitySection", () => () => null);
jest.mock("./ScriptTestPanel", () => () => null);

const apiClient = require("../../utils/apiClient").default;

const builtin = {
  name: "builtin",
  display_name: "Built-in pattern library",
  description: "Runs in-process.",
  redacts: true,
  local: true,
  available: true,
  detectors: [{ name: "injection", category: "injection", label: "Prompt injection" }],
  connection_fields: [],
};

const storedConfig = {
  provider: "builtin",
  detectors: [{ name: "injection" }],
  on_detect: "quarantine",
  scope: "last_user",
  tool_results: { scan: false, on_detect: "log" },
  quarantine: { placeholder: "[removed: {{source}}]" },
  cache: false,
};

const saved = {
  attributes: {
    name: "Injection",
    description: "",
    script: "",
    kind: "guardrail",
    response_filter: false,
    namespace: "",
    config: storedConfig,
  },
};

const renderEdit = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/filters/edit/7"]}>
        <Routes>
          <Route path="/admin/filters/edit/:id" element={<FilterForm />} />
          <Route path="/admin/filters" element={<div>filter list</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const savedBody = async () => {
  fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
  await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
  return apiClient.patch.mock.calls[0][1].data.attributes;
};

describe("FilterForm: tool-result guardrail settings (4uc.8)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    apiClient.get.mockImplementation((url) =>
      Promise.resolve(url === "/filters/7" ? { data: saved } : { data: [builtin] })
    );
    apiClient.patch.mockResolvedValue({ data: {} });
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("shows the stored tool-result settings and saves them unchanged", async () => {
    renderEdit();
    await screen.findByTestId("guardrail-detector-injection");
    expect(screen.getByTestId("guardrail-tool-results-scan")).not.toBeChecked();
    expect(screen.getByTestId("guardrail-tool-action")).toHaveValue("log");
    expect(screen.getByTestId("guardrail-quarantine-placeholder")).toHaveValue("[removed: {{source}}]");

    const attrs = await savedBody();
    expect(attrs.config).toEqual(storedConfig);
  });

  it("saves an edited override", async () => {
    renderEdit();
    await screen.findByTestId("guardrail-detector-injection");
    fireEvent.click(screen.getByTestId("guardrail-tool-results-scan"));
    fireEvent.change(screen.getByTestId("guardrail-tool-action"), { target: { value: "block" } });

    const attrs = await savedBody();
    expect(attrs.config.tool_results).toEqual({ on_detect: "block" });
  });

  it("turning it into a response filter replaces quarantine with block and saves no request-only settings", async () => {
    renderEdit();
    await screen.findByTestId("guardrail-detector-injection");
    fireEvent.click(screen.getByRole("checkbox", { name: /is this a response filter/i }));
    expect(screen.getByTestId("guardrail-action")).toHaveValue("block");

    const attrs = await savedBody();
    expect(attrs.response_filter).toBe(true);
    expect(attrs.config.on_detect).toBe("block");
    expect(attrs.config).not.toHaveProperty("tool_results");
    expect(attrs.config).not.toHaveProperty("quarantine");
    expect(attrs.config).not.toHaveProperty("cache");
    expect(attrs.config).not.toHaveProperty("budget");
  });

  // 4uc.61: a response filter on a tool runs on the tool's output; the help
  // text used to say response filters never run on tools.
  it("explains that a response filter runs on a tool's output", async () => {
    renderEdit();
    await screen.findByTestId("guardrail-detector-injection");
    expect(screen.getByText(/On a tool, it runs on the tool's output/)).toBeInTheDocument();
    expect(screen.queryByText(/not tools/)).not.toBeInTheDocument();
  });

  it("does not save a budget out of range", async () => {
    apiClient.get.mockImplementation((url) =>
      Promise.resolve(
        url === "/filters/7"
          ? { data: { attributes: { ...saved.attributes, config: { ...storedConfig, budget: { time_ms: 1 } } } } }
          : { data: [builtin] }
      )
    );
    renderEdit();
    await screen.findByTestId("guardrail-detector-injection");
    fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
    expect(await screen.findByText("Fix the budget values under Advanced")).toBeInTheDocument();
    expect(apiClient.patch).not.toHaveBeenCalled();
  });
});
