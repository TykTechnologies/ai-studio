import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import EmbedderPicker from "./EmbedderPicker";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

const rows = [
  { id: "1", attributes: { name: "OpenAI small", model: "text-embedding-3-small", vendor: "openai", privacy_score: 50 } },
  { id: "2", attributes: { name: "Local", model: "nomic", vendor: "ollama", privacy_score: 100 } },
];

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderPicker = (permissions, props = {}) => {
  const onChange = jest.fn();
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity(permissions)}>
          <EmbedderPicker value="" onChange={onChange} {...props} />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );
  return onChange;
};

describe("EmbedderPicker", () => {
  beforeEach(() => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/embedders") return Promise.resolve({ data: { data: rows } });
      if (url === "/embedders/vendors") return Promise.resolve({ data: { data: ["openai", "ollama"] } });
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it("lists embedders and reports the choice", async () => {
    const onChange = renderPicker(["embedders:read"]);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/embedders", expect.anything()));
    fireEvent.mouseDown(screen.getByRole("combobox", { name: /Embedder/ }));
    fireEvent.click(await within(screen.getByRole("listbox")).findByText("Local"));
    expect(onChange).toHaveBeenCalledWith("2", rows[1]);
  });

  it("offers inline creation only with embedders:write", async () => {
    renderPicker(["embedders:read"]);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "New embedder" })).not.toBeInTheDocument();
  });

  it("creates an embedder inline and selects it", async () => {
    const created = { id: "9", attributes: { name: "Fresh", model: "m", vendor: "openai", privacy_score: 60 } };
    apiClient.post.mockResolvedValue({ data: { data: created } });
    const onChange = renderPicker(["embedders:read", "embedders:write"], { minPrivacy: 60 });

    fireEvent.click(await screen.findByRole("button", { name: "New embedder" }));
    const dialog = await screen.findByTestId("embedder-create-dialog");
    fireEvent.change(within(dialog).getByTestId("embedder-dialog-name"), { target: { value: "Fresh" } });
    fireEvent.mouseDown(within(dialog).getByRole("combobox", { name: "API compatibility" }));
    fireEvent.click(await within(screen.getByRole("listbox")).findByText("OpenAI"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Create embedder" }));

    await waitFor(() => expect(onChange).toHaveBeenCalledWith("9", created));
    const sent = apiClient.post.mock.calls[0][1].data.attributes;
    expect(sent).toMatchObject({ name: "Fresh", vendor: "openai", model: "text-embedding-3-small", privacy_score: 60 });
  });

  it("flags an embedder below the required privacy level", async () => {
    renderPicker(["embedders:read"], { value: "1", minPrivacy: 70 });
    expect(await screen.findByText(/below 70; saving will be refused/)).toBeInTheDocument();
  });

  it("shows the saved embedder read-only without embedders:read", () => {
    renderPicker([], { value: "4", currentName: "Handbook embedder" });
    expect(screen.getByText("Handbook embedder")).toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/embedders", expect.anything());
  });
});
