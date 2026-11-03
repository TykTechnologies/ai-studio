import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import AppGuardrails from "./AppGuardrails";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
let mockEnterprise = true;
jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: mockEnterprise }),
}));
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: () => ({
    isFullAdmin: false,
    can: () => true,
    canAny: () => true,
    canAll: () => true,
  }),
}));

const apiClient = require("../../utils/apiClient").default;

// midsommar-clean-9u1j.6: one Guardrails section groups the capability, the
// exceptions and the prompt templates; it renders in the Community Edition
// too, with a notice.
describe("AppGuardrails", () => {
  beforeEach(() => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps/7/guardrail-capability") {
        return Promise.resolve({
          data: {
            data: {
              value: "",
              effective: "acting",
              suggestion: "read_only",
              reasons: [],
            },
          },
        });
      }
      if (url === "/apps/7/guardrail-exceptions")
        return Promise.resolve({
          data: { data: [], meta: { actions: {}, floors: {} } },
        });
      if (url === "/apps/7/prompt-templates")
        return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: [] });
    });
  });

  const renderSection = () =>
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <AppGuardrails appId="7" />
        </MemoryRouter>
      </ThemeProvider>,
    );

  it("groups capability, exceptions and templates", async () => {
    mockEnterprise = true;
    renderSection();
    expect(screen.getByText("Guardrails")).toBeInTheDocument();
    expect(await screen.findByTestId("capability-chip")).toHaveTextContent(
      "Not attested",
    );
    expect(await screen.findByText(/No exceptions/)).toBeInTheDocument();
    expect(await screen.findByText(/No prompt templates/)).toBeInTheDocument();
    expect(screen.queryByText(/Community Edition/)).not.toBeInTheDocument();
  });

  it("renders in the Community Edition with one notice", async () => {
    mockEnterprise = false;
    renderSection();
    expect(await screen.findByText(/No exceptions/)).toBeInTheDocument();
    expect(screen.getAllByText(/Community Edition/)).toHaveLength(1);
  });
});
