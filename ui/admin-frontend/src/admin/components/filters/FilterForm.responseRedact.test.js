import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import FilterForm from "./FilterForm";

// #722: on a response filter, a guardrail's Redact only logs. The form must
// say so as soon as the response-filter box is ticked, and when an existing
// response filter already uses Redact, without changing the saved action.

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
  detectors: [{ name: "pii", category: "pii", label: "Personal data" }],
  connection_fields: [],
};

const savedGuardrail = (responseFilter) => ({
  attributes: {
    name: "PII",
    description: "",
    script: "",
    kind: "guardrail",
    response_filter: responseFilter,
    namespace: "",
    config: {
      provider: "builtin",
      detectors: [{ name: "pii" }],
      on_detect: "redact",
      redaction: { style: "placeholder", placeholder: "" },
    },
  },
});

const renderEdit = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/filters/edit/7"]}>
        <Routes>
          <Route path="/admin/filters/edit/:id" element={<FilterForm />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const mockApi = (filter) => {
  apiClient.get.mockImplementation((url) =>
    Promise.resolve(url === "/filters/7" ? { data: filter } : { data: [builtin] })
  );
};

describe("FilterForm: guardrail Redact on a response filter (#722)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("warns as soon as the response-filter box is ticked, and stops warning when it is cleared", async () => {
    mockApi(savedGuardrail(false));
    renderEdit();
    await screen.findByTestId("guardrail-detector-pii");
    expect(screen.queryByTestId("guardrail-response-redact-warning")).not.toBeInTheDocument();
    expect(screen.getByText(/A finding the provider cannot rewrite/)).toBeInTheDocument();

    const toggle = screen.getByRole("checkbox", { name: /is this a response filter/i });
    fireEvent.click(toggle);
    expect(await screen.findByTestId("guardrail-response-redact-warning")).toHaveTextContent(
      /Redact does not change responses/
    );
    expect(screen.getByText(/Responses cannot be rewritten/)).toBeInTheDocument();

    fireEvent.click(toggle);
    expect(screen.queryByTestId("guardrail-response-redact-warning")).not.toBeInTheDocument();
  });

  it("labels Redact as logging only in the action menu of a response filter", async () => {
    mockApi(savedGuardrail(true));
    renderEdit();
    await screen.findByTestId("guardrail-detector-pii");
    fireEvent.mouseDown(screen.getByLabelText(/on detection/i));
    expect(await screen.findByRole("option", { name: "Redact (logs only on responses)" })).toBeInTheDocument();
  });

  it("opening a saved response filter that uses Redact shows the warning and keeps the action", async () => {
    mockApi(savedGuardrail(true));
    renderEdit();
    expect(await screen.findByTestId("guardrail-response-redact-warning")).toBeInTheDocument();
    expect(screen.getByTestId("guardrail-action")).toHaveValue("redact");
  });
});
