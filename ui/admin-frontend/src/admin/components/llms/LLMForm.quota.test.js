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

// At the licence's LLM limit a save is refused with 409 QUOTA_EXCEEDED; the
// form shows the server's message rather than "Failed to save LLM". The
// shared save error path is driven through an update here.

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
  "Your organisation has reached its licensed limit of 3 LLMs. Remove an LLM to make room, or contact your administrator to increase the quota.";

describe("LLMForm licence limit", () => {
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
    apiClient.patch.mockRejectedValue({
      response: { status: 409, data: { errors: [{ title: "Conflict", detail: DETAIL, code: "QUOTA_EXCEEDED" }] } },
    });
    Element.prototype.scrollIntoView = jest.fn();
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("shows the licence limit message", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <LLMForm />
        </MemoryRouter>
      </ThemeProvider>
    );
    await screen.findByDisplayValue("Open AI");
    fireEvent.click(screen.getByRole("button", { name: /update model provider|save|create/i }));

    expect(await screen.findByText(DETAIL)).toBeInTheDocument();
    expect(screen.queryByText("Failed to save LLM. Please try again.")).not.toBeInTheDocument();
  });
});
