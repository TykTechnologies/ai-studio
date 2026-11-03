import React from "react";
import { render, screen, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import FilterExceptions from "./FilterExceptions";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const apiClient = require("../../utils/apiClient").default;

// midsommar-clean-9u1j.6: a filter's owner sees which apps soften it.
describe("FilterExceptions", () => {
  beforeEach(() => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters/13/exceptions") {
        return Promise.resolve({
          data: {
            data: [
              {
                id: 1,
                app_id: 7,
                filter_id: 13,
                detector: "prompt_attack",
                channel: "text_user",
                action: "log",
                status: "approved",
                hit_count: 4,
              },
              {
                id: 2,
                app_id: 8,
                filter_id: 13,
                detector: "",
                channel: "tool_result",
                tool_name: "read",
                action: "quarantine",
                status: "proposed",
                hit_count: 0,
              },
              {
                id: 3,
                app_id: 9,
                filter_id: 13,
                detector: "",
                channel: "text_user",
                action: "log",
                status: "retired",
                hit_count: 2,
              },
            ],
          },
        });
      }
      if (url === "/apps/7")
        return Promise.resolve({
          data: { data: { attributes: { name: "Visor" } } },
        });
      return Promise.reject(new Error("no"));
    });
  });

  it("lists the filter's exceptions with links to their apps", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <FilterExceptions filterId="13" />
        </MemoryRouter>
      </ThemeProvider>,
    );
    const first = await screen.findByTestId("filter-exception-1");
    expect(await within(first).findByText("Visor")).toHaveAttribute(
      "href",
      "/admin/apps/7",
    );
    expect(first).toHaveTextContent("prompt_attack");
    expect(first).toHaveTextContent("Log only");
    expect(first).toHaveTextContent("4");
    // An app whose name cannot be read is named by id.
    expect(
      within(screen.getByTestId("filter-exception-2")).getByText("App #8"),
    ).toHaveAttribute("href", "/admin/apps/8");
    expect(screen.getByTestId("filter-exception-2")).toHaveTextContent(
      "Tool results (read)",
    );
    // Retired exceptions soften nothing and are left out.
    expect(screen.queryByTestId("filter-exception-3")).not.toBeInTheDocument();
  });

  it("says when no app softens the filter", async () => {
    apiClient.get.mockResolvedValue({ data: { data: [] } });
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <FilterExceptions filterId="14" />
        </MemoryRouter>
      </ThemeProvider>,
    );
    expect(
      await screen.findByText("No app softens this filter."),
    ).toBeInTheDocument();
  });
});
