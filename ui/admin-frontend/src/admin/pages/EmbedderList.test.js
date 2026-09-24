import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import EmbedderList from "./EmbedderList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const embedders = [
  { id: "1", attributes: { name: "Shared", model: "text-embedding-3-small", vendor: "openai", privacy_score: 60 } },
  { id: "2", attributes: { name: "Via LLM", model: "m", linked: true, llm_id: 3, llm_name: "Prod OpenAI", vendor: "openai", privacy_score: 90 } },
];

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity(permissions)}>
          <EmbedderList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("EmbedderList", () => {
  beforeEach(() => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/embedders") return Promise.resolve({ data: { data: embedders, meta: { total_count: 2, total_pages: 1 } } });
      if (url.endsWith("/dependents")) {
        return Promise.resolve({ data: { data: { attributes: { datasources: [{ id: 5, name: "Handbook" }], total: 1 } } } });
      }
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it("shows each embedder's connection", async () => {
    renderWith(["embedders:read"]);
    expect(await screen.findByText("Shared")).toBeInTheDocument();
    expect(screen.getByText("Prod OpenAI")).toBeInTheDocument();
    expect(screen.getByText("LLM provider")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add embedder" })).not.toBeInTheDocument();
  });

  it("surfaces the server's reason when a delete is refused", async () => {
    apiClient.delete.mockRejectedValue({
      response: { status: 409, data: { errors: [{ detail: "embedder is used by 1 datasource (Handbook)" }] } },
    });
    renderWith(["embedders:read", "embedders:write", "embedders:delete"]);
    await screen.findByText("Shared");
    const row = screen.getByRole("row", { name: /Shared/ });
    fireEvent.click(within(row).getByRole("button", { name: "Actions for Shared" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Delete embedder" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText(/Handbook/)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: /Delete/ }));
    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith("/embedders/1"));
    expect(await screen.findByText("embedder is used by 1 datasource (Handbook)")).toBeInTheDocument();
  });
});
