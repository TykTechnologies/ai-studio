import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import FilterDetails from "./FilterDetails";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../common/UsedBySection", () => () => null);

const apiClient = require("../../utils/apiClient").default;

const guardrail = (responseFilter) => ({
  id: "7",
  attributes: {
    name: "PII",
    kind: "guardrail",
    response_filter: responseFilter,
    config: {
      provider: "builtin",
      detectors: [{ name: "pii" }],
      on_detect: "redact",
      redaction: { style: "placeholder", placeholder: "[X]" },
    },
  },
});

const renderDetails = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/filters/7"]}>
        <Routes>
          <Route path="/admin/filters/:id" element={<FilterDetails />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("FilterDetails: guardrail action (#722)", () => {
  it("says a response filter's Redact only logs, and shows no redaction style", async () => {
    apiClient.get.mockResolvedValue({ data: guardrail(true) });
    renderDetails();
    expect(await screen.findByText(/Redact \(logs only on responses/)).toBeInTheDocument();
    expect(screen.queryByText("Redaction:")).not.toBeInTheDocument();
  });

  it("shows plain Redact and the style on a request filter", async () => {
    apiClient.get.mockResolvedValue({ data: guardrail(false) });
    renderDetails();
    expect(await screen.findByText("Redact")).toBeInTheDocument();
    expect(screen.getByText("Redaction:")).toBeInTheDocument();
  });
});

// 4uc.8: the tool-result settings, with the defaults a stored filter without
// tool_results gets.
describe("FilterDetails: tool results (4uc.8)", () => {
  const withConfig = (config, responseFilter = false) => ({
    id: "7",
    attributes: { name: "Injection", kind: "guardrail", response_filter: responseFilter, config },
  });

  it("shows that a Block filter without tool_results scans them and quarantines", async () => {
    apiClient.get.mockResolvedValue({
      data: withConfig({ provider: "builtin", detectors: [{ name: "injection" }], on_detect: "block" }),
    });
    renderDetails();
    expect(await screen.findByText("Scanned across the whole history")).toBeInTheDocument();
    expect(screen.getByText("Quarantine (default)")).toBeInTheDocument();
    expect(
      screen.getByText("[content removed by policy: {{detector}} detected in {{source}}]")
    ).toBeInTheDocument();
  });

  it("shows an explicit override and scanning turned off", async () => {
    apiClient.get.mockResolvedValue({
      data: withConfig({
        provider: "builtin",
        detectors: [{ name: "injection" }],
        on_detect: "log",
        tool_results: { scan: false },
        cache: false,
      }),
    });
    renderDetails();
    expect(await screen.findByText("Not scanned (only under all_messages)")).toBeInTheDocument();
    expect(screen.queryByText("Action on tool results:")).not.toBeInTheDocument();
    expect(screen.getByText("Off")).toBeInTheDocument();
  });

  it("shows no tool-result rows on a response filter", async () => {
    apiClient.get.mockResolvedValue({
      data: withConfig({ provider: "builtin", detectors: [{ name: "injection" }], on_detect: "block" }, true),
    });
    renderDetails();
    expect(await screen.findByText("Block")).toBeInTheDocument();
    expect(screen.queryByText("Tool results:")).not.toBeInTheDocument();
  });

  // 4uc.34: the stored budget holds only what the admin set, so the row lists
  // exactly those keys and is absent when none were set.
  const remote = (extra) => ({
    provider: "lakera",
    detectors: [{ name: "prompt_attack" }],
    on_detect: "block",
    connection: { api_key: "$SECRET/LAKERA" },
    ...extra,
  });

  it("lists only the budget keys the admin set", async () => {
    apiClient.get.mockResolvedValue({ data: withConfig(remote({ budget: { time_ms: 9000 } })) });
    renderDetails();
    expect(await screen.findByText("time_ms 9000")).toBeInTheDocument();
  });

  it("shows no budget row when none was set", async () => {
    apiClient.get.mockResolvedValue({ data: withConfig(remote({})) });
    renderDetails();
    expect(await screen.findByText("Scanned across the whole history")).toBeInTheDocument();
    expect(screen.queryByText("Budgets:")).not.toBeInTheDocument();
  });
});
