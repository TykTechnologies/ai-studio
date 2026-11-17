import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import GroupForm from "./GroupForm";
import GroupCatalogsDisplay from "./components/GroupCatalogsDisplay";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

// Workspaces E4: team responses list only the catalogs the caller can see,
// and a team save keeps the links to the others (server side). The team form
// must load and save with the visible subset, and tell a caller without a
// global catalog read that others are not listed; the team page likewise.

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../services/rbacService", () => ({
  __esModule: true,
  syncSubjectRoles: jest.fn(),
}));
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: { feature_portal: true, feature_chat: true, feature_gateway: true }, loading: false }),
}));
jest.mock("../../hooks/useOverviewData", () => ({
  __esModule: true,
  default: () => ({ getDocsLink: () => "" }),
}));
// The members section reports the stored membership once, as the real one does.
jest.mock("./components/GroupMembersSection", () => ({
  __esModule: true,
  default: function MockMembers({ onSelectedUsersChange }) {
    const { useEffect: useMockEffect } = jest.requireActual("react");
    useMockEffect(() => {
      onSelectedUsersChange([{ id: "11" }]);
    }, [onSelectedUsersChange]);
    return null;
  },
}));
jest.mock("./components/GroupRolesSection", () => ({ __esModule: true, default: () => null }));
jest.mock("./components/GroupPluginResourcesSection", () => ({ __esModule: true, default: () => null }));
jest.mock("../common/CollapsibleSection", () => require("../../../test-utils/component-mocks").collapsibleSectionMock);
jest.mock("../common/relationship-picker", () => require("../../../test-utils/component-mocks").relationshipPickerMock);

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({ id: "7" }),
}));

const { syncSubjectRoles } = require("../../services/rbacService");

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, ...attributes } });
// Manages teams globally; sees catalogs only in W1.
const scoped = identity({
  rbac_enabled: true,
  permissions: ["groups:read", "groups:write"],
  workspaces: [
    {
      id: 2,
      name: "W1",
      permissions: ["catalogues:read", "data-catalogues:read", "tool-catalogues:read"],
    },
  ],
});
const globalReader = identity({
  rbac_enabled: true,
  permissions: ["groups:read", "groups:write", "catalogues:read", "data-catalogues:read", "tool-catalogues:read"],
  workspaces: [],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const visibleLLMCatalog = { id: "1", attributes: { name: "W1 LLM catalog", workspace_id: 2 } };

// The team links more catalogs than these; the response holds only the
// visible one (and no data or tool catalogs at all).
const team = {
  data: {
    id: "7",
    type: "Group",
    attributes: {
      name: "Support",
      catalogues: [visibleLLMCatalog],
      data_catalogues: [],
      tool_catalogues: [],
      roles: [],
    },
  },
};

const list = (rows) => Promise.resolve({ data: { data: rows }, headers: {} });

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <GroupForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("GroupForm with catalogs the caller cannot see", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    syncSubjectRoles.mockResolvedValue({ failed: [] });
    apiClient.get.mockImplementation((url) => {
      if (url === "/groups/7") return Promise.resolve({ data: team });
      if (url === "/groups/7/users") return list([{ id: "11" }]);
      if (url === "/catalogues") return list([visibleLLMCatalog]);
      return list([]);
    });
    apiClient.patch.mockResolvedValue({ data: team });
  });

  it("loads, notes the hidden catalogs, and saves the visible ones", async () => {
    renderForm(scoped);
    const picker = await screen.findByLabelText("Model provider catalogs");
    expect(within(picker).getByText("W1 LLM catalog")).toBeInTheDocument();
    expect(screen.getAllByTestId("other-workspaces-note")).toHaveLength(3);
    expect(screen.getAllByTestId("other-workspaces-note")[0]).toHaveTextContent(
      "Model provider catalogs in Workspaces you cannot see are not listed; saving keeps this team's links to them.",
    );

    fireEvent.click(screen.getByRole("button", { name: "Update team" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [url, body] = apiClient.patch.mock.calls[0];
    expect(url).toBe("/groups/7");
    expect(body.data.attributes).toEqual(
      expect.objectContaining({ name: "Support", catalogues: [1], data_catalogues: [], tool_catalogues: [], members: [11] }),
    );
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/groups"));
  });

  it("a caller who sees no catalogs is not told there are none", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/groups/7") {
        return Promise.resolve({ data: { data: { ...team.data, attributes: { ...team.data.attributes, catalogues: [] } } } });
      }
      if (url === "/groups/7/users") return list([{ id: "11" }]);
      return list([]);
    });
    renderForm(scoped);
    expect(await screen.findByText(/There are no catalogs in your Workspaces/)).toBeInTheDocument();
    expect(screen.queryByText(/Currently, there are no catalogs available/)).not.toBeInTheDocument();
  });

  it("a global catalog reader sees no note", async () => {
    renderForm(globalReader);
    await screen.findByLabelText("Model provider catalogs");
    expect(screen.queryByTestId("other-workspaces-note")).not.toBeInTheDocument();
  });

  it("Community: no note", async () => {
    renderForm(community);
    await screen.findByLabelText("Model provider catalogs");
    expect(screen.queryByTestId("other-workspaces-note")).not.toBeInTheDocument();
  });
});

describe("GroupCatalogsDisplay with catalogs the caller cannot see", () => {
  const features = { feature_portal: true, feature_chat: true };
  const renderDisplay = (who, props) =>
    render(
      <ThemeProvider theme={testTheme}>
        <PermissionsProvider identity={who}>
          <GroupCatalogsDisplay features={features} {...props} />
        </PermissionsProvider>
      </ThemeProvider>,
    );

  beforeEach(() => {
    clearIdentity();
  });

  it("notes each catalog kind the caller cannot read globally", () => {
    renderDisplay(scoped, { catalogues: [visibleLLMCatalog] });
    expect(screen.getByText("W1 LLM catalog")).toBeInTheDocument();
    expect(screen.getAllByTestId("other-workspaces-note")).toHaveLength(3);
  });

  it("an empty list does not read as the team having no catalogs", () => {
    renderDisplay(scoped, {});
    expect(screen.getByText(/No catalogs in your Workspaces are assigned to this team/)).toBeInTheDocument();
    expect(screen.queryByText("No catalogs are currently assigned to this team.")).not.toBeInTheDocument();
  });

  it("a global reader and Community see the plain lists", () => {
    const { unmount } = renderDisplay(globalReader, {});
    expect(screen.getByText("No catalogs are currently assigned to this team.")).toBeInTheDocument();
    unmount();
    clearIdentity();
    renderDisplay(community, { catalogues: [visibleLLMCatalog] });
    expect(screen.queryByTestId("other-workspaces-note")).not.toBeInTheDocument();
  });
});
