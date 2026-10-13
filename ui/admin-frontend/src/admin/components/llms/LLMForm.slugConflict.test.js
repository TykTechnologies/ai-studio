import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
import { useEdition } from "../../context/EditionContext";

// #785: a name whose slug another LLM already has is refused with 400 and a
// detail; the form shows it under Name rather than "Failed to save LLM".

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../hooks/useSystemFeatures", () => ({ __esModule: true, default: () => ({ features: {} }) }));
jest.mock("../../services/pluginService", () => ({
  __esModule: true,
  default: { listPlugins: jest.fn(), getAvailableHookTypes: jest.fn(), getHookTypeLabel: jest.fn() },
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
  useParams: () => ({ id: "5" }),
}));

const DETAIL =
  'LLM name "Open AI" is too close to LLM 2 ("OpenAI"): both have the slug "open-ai", and LLM names must differ by more than case, spaces or punctuation';

describe("LLMForm slug conflict", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: false });
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/llms/5") {
        return Promise.resolve({
          data: {
            data: {
              id: "5",
              attributes: {
                name: "Open AI", vendor: "openai", api_endpoint: "https://api.openai.com/v1", api_key: "[redacted]",
                privacy_score: 50, active: true, filters: [], plugins: [], allowed_models: [], default_model: "gpt-4o", namespace: "",
              },
            },
          },
        });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.patch.mockRejectedValue({ response: { status: 400, data: { errors: [{ title: "Bad Request", detail: DETAIL }] } } });
    Element.prototype.scrollIntoView = jest.fn();
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("shows the conflict under the Name field and focuses it", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <LLMForm />
        </MemoryRouter>
      </ThemeProvider>
    );
    const name = await screen.findByDisplayValue("Open AI");
    fireEvent.click(screen.getByRole("button", { name: /update llm|save|create/i }));

    await waitFor(() => expect(name).toHaveAttribute("aria-invalid", "true"));
    expect(screen.getAllByText(DETAIL).length).toBeGreaterThan(0);
    expect(screen.queryByText("Failed to save LLM. Please try again.")).not.toBeInTheDocument();
    await waitFor(() => expect(name).toHaveFocus());
  });
});
