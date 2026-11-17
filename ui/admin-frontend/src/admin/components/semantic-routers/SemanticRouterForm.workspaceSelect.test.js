import React from "react";
import { render, screen, fireEvent, waitFor, within, configure } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import SemanticRouterForm from "./SemanticRouterForm";
import { useEdition } from "../../context/EditionContext";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

// Role queries skip the visibility check on this large form (see
// SemanticRouterForm.test.js).
configure({ defaultHidden: true });

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("../../context/EditionContext");
jest.mock("../common/relationship-picker", () =>
  require("../../../test-utils/component-mocks").relationshipPickerMock,
);
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

// Workspaces E4: the semantic router form's Workspace select (landing on
// create, move on edit), its errors, and a router the caller cannot see.

const llms = [{ id: "1", attributes: { name: "OpenAI", vendor: "openai" } }];

const storedRouter = {
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Smart",
        slug: "smart",
        description: "",
        active: false,
        namespace: "",
        short_description: "",
        long_description: "",
        logo_url: "",
        catalogues: [],
        workspace_id: 2,
        settings: {
          mode: "enforce",
          allow_explicit_route: false,
          input_scope: "last_user",
          judge: { enabled: false, model_ref: { llm_id: 0, model: "" }, when: "low_confidence" },
          affinity: { enabled: false },
          default_route: "simple",
        },
        routes: [{ name: "simple", description: "", target: { type: "llm", llm_id: 1, model: "gpt-4o-mini" } }],
      },
    },
  },
};

const perms = ["semantic-routers:read", "semantic-routers:write", "semantic-routers:publish"];
const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({ rbac_enabled: true, workspaces: [{ id: 2, name: "W1", permissions: perms }] });
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: perms },
    { id: 3, name: "W2", permissions: perms },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <SemanticRouterForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const choose = (label, option, container = document.body) => {
  const select = within(container).getByLabelText(label, { selector: "[role=combobox]" });
  fireEvent.mouseDown(select);
  const labels = (select.getAttribute("aria-labelledby") || "").split(" ");
  const listbox = screen.getAllByRole("listbox").find((lb) => labels.includes(lb.getAttribute("aria-labelledby")));
  fireEvent.click(within(listbox).getByText(option));
};

// A minimal valid router: one LLM route, which is also the default.
const fillRequired = async () => {
  await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/llms", expect.anything()));
  await screen.findAllByTestId("relationship-picker");
  fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Smart" } });
  fireEvent.click(screen.getByText("Add Route"));
  const card = screen.getByTestId("route-card-0");
  fireEvent.change(within(card).getByLabelText(/^Route name/), { target: { value: "simple" } });
  choose("Model provider", "OpenAI (openai)", card);
  fireEvent.change(within(card).getByLabelText(/^Model(?! provider)/), { target: { value: "gpt-4o-mini" } });
  choose("Default route", "simple");
};

const sentAttributes = (method) =>
  apiClient[method].mock.calls.find((c) => String(c[0]).startsWith("/semantic-routers"))[1].data.attributes;

describe("SemanticRouterForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms") return Promise.resolve({ data: { data: llms }, headers: {} });
      if (url === "/semantic-routers/5") return Promise.resolve(storedRouter);
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9", attributes: {} } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5", attributes: {} } } });
    apiClient.put.mockResolvedValue({ data: { data: { catalogues: [] } } });
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    await fillRequired();
    fireEvent.click(screen.getByText("Create"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post").workspace_id).toBe(2);
  }, 15000);

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderForm(scopedTwo);
    await screen.findByTestId("workspace-select");
    await fillRequired();
    fireEvent.click(screen.getByText("Create"));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  }, 15000);

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "5" };
    renderForm(scopedTwo);
    await screen.findByDisplayValue("Smart");
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByText("Update"));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    choose("Workspace", "W2");
    fireEvent.click(screen.getByText("Update"));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  }, 15000);

  it("says catalogs in other Workspaces are kept when the caller cannot read every catalog", async () => {
    mockParams = { id: "5" };
    renderForm(scopedW1);
    await screen.findByDisplayValue("Smart");
    expect(screen.getByTestId("router-catalogues-hidden-note")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
  });

  it("a router the caller cannot see (404) renders the not-found panel", async () => {
    mockParams = { id: "5" };
    apiClient.get.mockImplementation((url) =>
      url === "/semantic-routers/5"
        ? Promise.reject({ response: { status: 404, data: {} } })
        : Promise.resolve({ data: { data: [] }, headers: {} }),
    );
    renderForm(scopedW1);
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });

  it("Community: no Workspace select, no catalog note and no workspace_id sent", async () => {
    renderForm(community);
    await fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    expect(screen.queryByTestId("router-catalogues-hidden-note")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Create"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  }, 15000);
});
