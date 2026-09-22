import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import LLMList from "./LLMList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const llms = [
  {
    id: "1",
    attributes: { name: "Prod OpenAI", short_description: "main", vendor: "openai", privacy_score: 2, active: true, credential_status: "ok" },
  },
  {
    id: "2",
    attributes: { name: "Staging Claude", short_description: "staging", vendor: "anthropic", privacy_score: 2, active: false, credential_status: "ok" },
  },
];

// A real PermissionsProvider with an identity that holds exactly the listed
// permissions, so "publish never implies write" is exercised for real.
const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity(permissions)}>
          <LLMList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  const row = (await screen.findByText(name)).closest("tr");
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("LLMList permission gating (llms:publish vs llms:write)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockResolvedValue({
      data: { data: llms },
      headers: { "x-total-count": "2", "x-total-pages": "1" },
    });
    apiClient.post.mockResolvedValue({ data: {} });
    apiClient.patch.mockResolvedValue({ data: {} });
  });

  it("publish-only: shows the actions column with just the toggle, and bulk activate/deactivate without delete", async () => {
    renderWith(["llms:publish"]);
    await screen.findByText("Prod OpenAI");

    expect(screen.queryByRole("button", { name: /Add LLM provider/ })).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: /Actions/ })).toBeInTheDocument();

    const menu = await openRowMenu("Prod OpenAI");
    expect(within(menu).getByRole("menuitem", { name: "Deactivate LLM provider" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Edit LLM provider/ })).not.toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Delete LLM provider/ })).not.toBeInTheDocument();

    // The toggle goes through the publish-gated endpoint, not PATCH /llms/:id.
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Deactivate LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms/1/deactivate"));
    expect(apiClient.patch).not.toHaveBeenCalled();

    // Bulk selection is available and offers activate/deactivate but no delete.
    fireEvent.click(screen.getByRole("checkbox", { name: "Select Prod OpenAI" }));
    const toolbar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    expect(within(toolbar).getByRole("button", { name: "Activate" })).toBeInTheDocument();
    expect(within(toolbar).getByRole("button", { name: "Deactivate" })).toBeInTheDocument();
    expect(within(toolbar).queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("publish-only: activating an inactive provider posts to /activate", async () => {
    renderWith(["llms:publish"]);
    const menu = await openRowMenu("Staging Claude");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Activate LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms/2/activate"));
  });

  it("write-only: offers edit and delete but not the toggle", async () => {
    renderWith(["llms:write"]);
    await screen.findByText("Prod OpenAI");

    expect(screen.getByRole("button", { name: /Add LLM provider/ })).toBeInTheDocument();
    const menu = await openRowMenu("Prod OpenAI");
    expect(within(menu).getByRole("menuitem", { name: "Edit LLM provider" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete LLM provider" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Deactivate LLM provider/ })).not.toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Activate LLM provider/ })).not.toBeInTheDocument();

    fireEvent.keyDown(menu, { key: "Escape" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Select Prod OpenAI" }));
    const toolbar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    expect(within(toolbar).getByRole("button", { name: "Delete" })).toBeInTheDocument();
    expect(within(toolbar).queryByRole("button", { name: "Activate" })).not.toBeInTheDocument();
  });

  it("read-only: no actions column and no selection", async () => {
    renderWith(["llms:read"]);
    await screen.findByText("Prod OpenAI");
    expect(screen.queryByRole("columnheader", { name: /Actions/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Select Prod OpenAI" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Add LLM provider/ })).not.toBeInTheDocument();
  });
});

describe("LLMList vendor cell accessible name", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockResolvedValue({
      data: { data: llms },
      headers: { "x-total-count": "2", "x-total-pages": "1" },
    });
  });

  it("reads the vendor name once: the logo is decorative", async () => {
    renderWith(["llms:read"]);
    const row = (await screen.findByText("Prod OpenAI")).closest("tr");
    const vendorCell = within(row).getByText("OpenAI").closest("td");
    expect(vendorCell.textContent).toBe("OpenAI");
    // An <img alt=""> is presentational: no accessible name to double up.
    expect(within(vendorCell).getByRole("presentation")).toHaveAttribute("alt", "");
    // Nothing in the row announces "OpenAI" a second time.
    expect(within(row).queryByRole("img", { name: /OpenAI/ })).not.toBeInTheDocument();
  });
});
