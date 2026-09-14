import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import Secrets from "./Secrets";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), delete: jest.fn() },
}));
jest.mock("../components/rbac/Can", () => ({ children }) => <>{children}</>);
jest.mock("../../components/common/Icon", () => (props) => <div data-testid="mock-icon">{props.name}</div>);
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

describe("Secrets list", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({
      data: {
        data: [
          {
            id: "1",
            attributes: {
              var_name: "OPENAI_KEY",
              has_value: true,
              referenced_by: [
                { type: "llm", id: 4, name: "OpenAI Prod" },
                { type: "llm", id: 5, name: "OpenAI Dev" },
              ],
            },
          },
          { id: "2", attributes: { var_name: "EMPTY_KEY", has_value: false, referenced_by: [] } },
        ],
      },
      headers: { "x-total-count": "2", "x-total-pages": "1" },
    });
  });

  it("shows whether each secret has a value and which LLMs reference it", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <Secrets />
        </MemoryRouter>
      </ThemeProvider>,
    );
    const filled = (await screen.findByText("OPENAI_KEY")).closest("tr");
    expect(screen.getByText("Value")).toBeInTheDocument();
    expect(screen.getByText("Used by")).toBeInTheDocument();
    expect(within(filled).getByText("Set")).toBeInTheDocument();
    const link = within(filled).getByRole("link", { name: "OpenAI Prod" });
    expect(link).toHaveAttribute("href", "/admin/llms/4");
    expect(within(filled).getByRole("link", { name: "OpenAI Dev" })).toHaveAttribute("href", "/admin/llms/5");

    const empty = screen.getByText("EMPTY_KEY").closest("tr");
    await waitFor(() => expect(within(empty).getByText("Empty")).toBeInTheDocument());
    expect(within(empty).getByText("Not referenced")).toBeInTheDocument();
  });
});
