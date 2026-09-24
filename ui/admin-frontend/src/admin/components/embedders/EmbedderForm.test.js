import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import EmbedderForm, { lockedReasonFor } from "./EmbedderForm";
import apiClient from "../../utils/apiClient";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const saved = {
  id: "1",
  attributes: { name: "Shared", vendor: "openai", endpoint: "https://api.openai.com/v1", api_key: "[redacted]", has_api_key: true, model: "text-embedding-3-small", privacy_score: 60 },
};

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <EmbedderForm />
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("EmbedderForm", () => {
  beforeEach(() => {
    mockParams = { id: "1" };
    apiClient.get.mockImplementation((url) => {
      if (url === "/embedders/1") return Promise.resolve({ data: { data: saved } });
      if (url === "/embedders/1/dependents") {
        return Promise.resolve({ data: { data: { attributes: { datasources: [{ id: 5, name: "Handbook" }], total: 1 } } } });
      }
      if (url === "/embedders/vendors") return Promise.resolve({ data: { data: ["openai", "ollama"] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.patch.mockResolvedValue({ data: { data: saved } });
  });

  it("locks the model while data sources use it and keeps the key on save", async () => {
    renderForm();
    expect(await screen.findByText(/Used by Handbook/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("embedder-model")).toHaveValue("text-embedding-3-small"));
    expect(screen.getByTestId("embedder-model")).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Endpoint URL"), { target: { value: "https://proxy/v1" } });
    fireEvent.click(screen.getByRole("button", { name: "Save embedder" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const attrs = apiClient.patch.mock.calls[0][1].data.attributes;
    expect(attrs).toMatchObject({ endpoint: "https://proxy/v1", api_key: "[redacted]", model: "text-embedding-3-small" });
    expect(mockNavigate).toHaveBeenCalledWith("/admin/embedders/1");
  });

  it("shows the server's refusal", async () => {
    apiClient.patch.mockRejectedValue({ response: { status: 409, data: { errors: [{ detail: "cannot change while datasources use it" }] } } });
    renderForm();
    await waitFor(() => expect(screen.getByTestId("embedder-model")).toHaveValue("text-embedding-3-small"));
    fireEvent.click(screen.getByRole("button", { name: "Save embedder" }));
    expect(await screen.findByText("cannot change while datasources use it")).toBeInTheDocument();
  });

  it("explains the lock only when data sources use the embedder", () => {
    expect(lockedReasonFor({ datasources: [] })).toBe("");
    expect(lockedReasonFor(null)).toBe("");
    expect(lockedReasonFor({ datasources: [{ name: "A" }, { name: "B" }] })).toMatch(/^Used by A, B\./);
  });
});
