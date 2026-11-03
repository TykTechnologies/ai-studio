import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import FilterForm from "./FilterForm";

// midsommar-clean-9u1j.13: the judge is saved under config.judge, and the
// server's refusal of it is shown in the judge section.

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

const saved = (judge) => ({
  attributes: {
    name: "Wolf",
    description: "",
    script: "",
    kind: "guardrail",
    response_filter: false,
    namespace: "",
    config: { provider: "builtin", detectors: [{ name: "injection" }], on_detect: "block", ...(judge ? { judge } : {}) },
  },
});

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

describe("FilterForm: second opinion", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  const serve = (judge) =>
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters/7") return Promise.resolve({ data: saved(judge) });
      if (url === "/llms") {
        return Promise.resolve({
          data: { data: [{ id: "70", attributes: { name: "Judge", vendor: "openai", default_model: "m", active: true } }] },
          headers: {},
        });
      }
      return Promise.resolve({ data: [builtin] });
    });

  test("a stored judge is saved unchanged", async () => {
    serve({ mode: "record", llm_id: 70, model: "m", timeout_ms: 3000 });
    apiClient.patch.mockResolvedValue({ data: {} });
    renderEdit();
    expect(await screen.findByText("Second opinion (LLM judge): Record only")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(apiClient.patch.mock.calls[0][1].data.attributes.config.judge).toEqual({
      mode: "record", llm_id: 70, model: "m", timeout_ms: 3000,
    });
  });

  test("the server's refusal of the judge is shown in its section", async () => {
    serve({ mode: "record", llm_id: 70 });
    const detail = 'invalid filter: judge LLM "Judge" has privacy score 10, below the 90 of LLM "On-prem" whose traffic it would read';
    apiClient.patch.mockRejectedValue({ response: { data: { errors: [{ detail }] } } });
    renderEdit();
    await screen.findByText("Second opinion (LLM judge): Record only");
    fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
    expect(await screen.findByTestId("guardrail-judge-error")).toHaveTextContent("privacy score 10");
  });

  test("a judge without an LLM is not sent", async () => {
    serve({ mode: "record" });
    renderEdit();
    await screen.findByText("Second opinion (LLM judge): Record only");
    fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
    expect(await screen.findByTestId("guardrail-judge-error")).toHaveTextContent("Choose the LLM");
    expect(apiClient.patch).not.toHaveBeenCalled();
  });
});
