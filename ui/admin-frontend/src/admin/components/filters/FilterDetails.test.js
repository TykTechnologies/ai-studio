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
