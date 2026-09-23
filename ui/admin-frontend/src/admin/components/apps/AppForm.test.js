import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import AppForm from "./AppForm";
import apiClient, { appToolAPI } from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import {
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../../../components/unsaved-changes";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
  appToolAPI: { listAvailableTools: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: {} }),
}));
jest.mock("../../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});
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

// Option lists are ordered so that the mock picker's "Add" (which appends
// options[0]) picks something that is not already selected.
const users = [{ id: "1", attributes: { name: "Test Admin" } }];
const llms = [
  { id: "2", attributes: { name: "Anthropic" } },
  { id: "1", attributes: { name: "OpenAI" } },
];
const datasources = [{ id: "3", attributes: { name: "Docs" } }];
const tools = [
  { id: "4", attributes: { name: "Weather" } },
  { id: "5", attributes: { name: "Time" } },
];
const resourceTypes = [
  { plugin_id: 3, slug: "vector-db", name: "Vector DBs", has_privacy_score: true, access_granted_via_app: true },
  // Catalog-style types: an app credential unlocks none of their instances.
  { plugin_id: 9, slug: "agent", name: "Agents", access_granted_via_app: false },
  { plugin_id: 9, slug: "prompt", name: "Prompts", access_granted_via_app: false },
];
const instances = [{ id: "i1", name: "Pinecone", privacy_score: 40, access_granted_via_app: true }];
const agentInstances = [{ id: "a1", name: "Agent One", access_granted_via_app: false }];

const appPayload = () => ({
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Sales bot",
        description: "Answers sales questions",
        user_id: 1,
        llm_ids: [1],
        datasource_ids: [],
        tool_ids: [5],
        plugin_resources: [
          { plugin_id: 3, resource_type_slug: "vector-db", instance_ids: ["i1"] },
        ],
        monthly_budget: null,
        budget_start_date: null,
        namespace: "",
        metadata: {},
        credential_id: 9,
      },
    },
  },
});

const credentialPayload = (active) => ({
  data: { data: { id: "9", attributes: { key_id: "key", secret: "s3cret", active } } },
});

const DirtyProbe = () => {
  const { isDirty } = useUnsavedChanges();
  return <span data-testid="registry-dirty">{String(isDirty)}</span>;
};

const renderForm = ({ withGuard = false } = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        {withGuard ? (
          <UnsavedChangesProvider>
            <AppForm />
            <DirtyProbe />
          </UnsavedChangesProvider>
        ) : (
          <AppForm />
        )}
      </MemoryRouter>
    </ThemeProvider>,
  );

const picker = (itemLabel) =>
  screen.getAllByTestId("relationship-picker").find((el) => el.dataset.itemLabel === itemLabel);

const pickerItems = (itemLabel) =>
  within(picker(itemLabel))
    .queryAllByTestId("relationship-picker-item")
    .map((el) => el.textContent);

const waitForLoaded = async () => {
  await screen.findByDisplayValue("Sales bot");
  await waitFor(() => expect(pickerItems("LLM provider")).toEqual(["OpenAI"]));
  await waitFor(() => expect(pickerItems("vector dbs")).toEqual(["Pinecone"]));
};

describe("AppForm relationships and commit semantics", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "5" };
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/users") return Promise.resolve({ data: { data: users } });
      if (url === "/llms") return Promise.resolve({ data: { data: llms } });
      if (url === "/datasources") return Promise.resolve({ data: { data: datasources } });
      if (url === "/apps/5") return Promise.resolve(appPayload());
      if (url === "/credentials/9") return Promise.resolve(credentialPayload(false));
      if (url === "/plugin-resource-types") return Promise.resolve({ data: { data: resourceTypes } });
      if (url === "/plugin-resource-types/3/vector-db/instances") {
        return Promise.resolve({ data: { data: instances } });
      }
      if (url === "/plugin-resource-types/9/agent/instances") {
        return Promise.resolve({ data: { data: agentInstances } });
      }
      if (url === "/plugin-resource-types/9/prompt/instances") {
        return Promise.resolve({ data: { data: [] } });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    appToolAPI.listAvailableTools.mockResolvedValue({ data: { data: tools } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    apiClient.post.mockResolvedValue({ data: { data: { id: "6" } } });
  });

  // The multi-selects with chips were one of five idioms for "which X belong
  // to Y" (UX review F-05). Each relationship is now a compact picker.
  it("renders one compact picker per relationship with the loaded members", async () => {
    renderForm();
    await waitForLoaded();

    expect(picker("LLM provider")).toHaveAttribute("data-variant", "compact");
    expect(picker("data source")).toHaveAttribute("data-variant", "compact");
    expect(picker("tool")).toHaveAttribute("data-variant", "compact");
    expect(pickerItems("data source")).toEqual([]);
    expect(pickerItems("tool")).toEqual(["Time"]);
    // Every picker offers the full option list (the picker itself hides
    // what is already selected).
    expect(within(picker("LLM provider")).getByTestId("relationship-picker-options-count")).toHaveTextContent("2");
    expect(within(picker("tool")).getByTestId("relationship-picker-options-count")).toHaveTextContent("2");
    expect(within(picker("vector dbs")).getByTestId("relationship-picker-options-count")).toHaveTextContent("1");
    expect(screen.queryByRole("combobox", { name: /LLM providers/ })).not.toBeInTheDocument();
  });

  // Types an app credential does not unlock get no picker, unless the app
  // already holds a binding from before the type was classified: that stays
  // visible, removable, and is sent back unchanged.
  it("hides non-app-granted resource types but keeps an app's existing bindings", async () => {
    renderForm();
    await waitForLoaded();
    expect(picker("agents")).toBeUndefined();
    expect(picker("prompts")).toBeUndefined();
  });

  it("shows a legacy binding to a non-app-granted type as a removable chip and resends it unchanged", async () => {
    const legacy = appPayload();
    legacy.data.data.attributes.plugin_resources.push({ plugin_id: 9, resource_type_slug: "agent", instance_ids: ["legacy-agent"] });
    const base = apiClient.get.getMockImplementation();
    apiClient.get.mockImplementation((url) => (url === "/apps/5" ? Promise.resolve(legacy) : base(url)));

    renderForm();
    await waitForLoaded();
    await waitFor(() => expect(pickerItems("agents")).toEqual(["legacy-agent"]));
    expect(within(picker("agents")).getByTestId("relationship-picker-options-count")).toHaveTextContent("0");

    fireEvent.click(screen.getByRole("button", { name: "Update app" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, body] = apiClient.patch.mock.calls[0];
    expect(body.data.attributes.plugin_resources).toEqual([
      { plugin_id: 3, resource_type_slug: "vector-db", instance_ids: ["i1"] },
      { plugin_id: 9, resource_type_slug: "agent", instance_ids: ["legacy-agent"] },
    ]);
  });

  // Model routers (Enterprise) are granted like LLM providers; the picker
  // only shows when there are routers or the app already holds one.
  it("binds model routers through model_router_ids", async () => {
    const withRouter = appPayload();
    withRouter.data.data.attributes.model_router_ids = [21];
    withRouter.data.data.attributes.model_routers = [{ id: 21, name: "Prod router", slug: "prod" }];
    const base = apiClient.get.getMockImplementation();
    apiClient.get.mockImplementation((url, config) => {
      if (url === "/apps/5") return Promise.resolve(withRouter);
      if (url === "/model-routers") {
        return Promise.resolve({
          data: {
            data: [
              { id: "22", attributes: { name: "Batch router" } },
              { id: "21", attributes: { name: "Prod router" } },
            ],
          },
        });
      }
      return base(url, config);
    });

    renderForm();
    await waitForLoaded();
    await waitFor(() => expect(pickerItems("model router")).toEqual(["Prod router"]));
    fireEvent.click(within(picker("model router")).getByTestId("relationship-picker-add"));

    fireEvent.click(screen.getByRole("button", { name: "Update app" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(apiClient.patch.mock.calls[0][1].data.attributes.model_router_ids).toEqual([21, 22]);
  });

  it("hides the model router picker when there are none", async () => {
    renderForm();
    await waitForLoaded();
    expect(picker("model router")).toBeUndefined();
  });

  it("saves the picker selections in the unchanged id-array payload", async () => {
    renderForm();
    await waitForLoaded();

    fireEvent.click(within(picker("LLM provider")).getByTestId("relationship-picker-add"));
    fireEvent.click(within(picker("data source")).getByTestId("relationship-picker-add"));
    fireEvent.click(within(picker("tool")).getByTestId("relationship-picker-remove"));
    expect(pickerItems("LLM provider")).toEqual(["OpenAI", "Anthropic"]);

    fireEvent.click(screen.getByRole("button", { name: "Update app" }));

    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [url, body] = apiClient.patch.mock.calls[0];
    expect(url).toBe("/apps/5");
    expect(body.data.attributes.llm_ids).toEqual([1, 2]);
    expect(body.data.attributes.datasource_ids).toEqual([3]);
    expect(body.data.attributes.tool_ids).toEqual([]);
    expect(body.data.attributes.plugin_resources).toEqual([
      { plugin_id: 3, resource_type_slug: "vector-db", instance_ids: ["i1"] },
    ]);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps", expect.anything());
  });

  // F-03: the credential switch commits on click while the rest of the form
  // waits for Update. It stays that way, but says so, and must not count as
  // an unsaved change.
  it("labels the credential Active switch as immediate and keeps it out of the dirty state", async () => {
    renderForm({ withGuard: true });
    await waitForLoaded();
    await waitFor(() => expect(screen.getByTestId("credential-active-caption")).toHaveTextContent("Applies immediately"));
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.click(screen.getByRole("button", { name: "Credential Information" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Credentials active (approved)", hidden: true }));

    await waitFor(() =>
      expect(apiClient.patch).toHaveBeenCalledWith("/credentials/9", {
        data: { type: "credentials", attributes: { active: true } },
      }),
    );
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });

  it("has a Cancel that returns to the list when clean, and registers edits with the guard", async () => {
    renderForm({ withGuard: true });
    await waitForLoaded();

    const cancel = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancel);
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps");
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.click(within(picker("LLM provider")).getByTestId("relationship-picker-add"));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));

    mockNavigate.mockClear();
    fireEvent.click(cancel);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("starts clean on create and marks the form saved before redirecting", async () => {
    mockParams = {};
    renderForm({ withGuard: true });
    await waitFor(() => expect(picker("LLM provider")).toBeTruthy());
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "New app" } });
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));
    fireEvent.mouseDown(screen.getByLabelText(/User/));
    fireEvent.click(await screen.findByRole("option", { name: "Test Admin" }));
    fireEvent.click(within(picker("tool")).getByTestId("relationship-picker-add"));

    fireEvent.click(screen.getByRole("button", { name: "Add app" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(apiClient.post.mock.calls[0][1].data.attributes.tool_ids).toEqual([4]);
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/apps", expect.anything()));
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });
});

describe("AppForm team budgets (Enterprise)", () => {
  const teamReport = {
    team_id: 3,
    enabled: true,
    managed: true,
    monthly_budget: 100,
    default_app_allocation: 20,
    unallocated: 40,
  };

  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "5" };
    useEdition.mockReturnValue({ isEnterprise: true });
    const payload = appPayload();
    Object.assign(payload.data.data.attributes, { team_id: 3, monthly_budget: 20 });
    apiClient.get.mockImplementation((url) => {
      if (url === "/users") return Promise.resolve({ data: { data: users } });
      if (url === "/groups") {
        return Promise.resolve({
          data: { data: [{ id: "3", attributes: { name: "Engineering" } }, { id: "4", attributes: { name: "Ops" } }] },
        });
      }
      if (url === "/groups/3/budget") return Promise.resolve({ data: teamReport });
      if (url === "/apps/5") return Promise.resolve(payload);
      if (url === "/credentials/9") return Promise.resolve(credentialPayload(true));
      if (url === "/plugin-resource-types") return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    appToolAPI.listAvailableTools.mockResolvedValue({ data: { data: tools } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
  });

  it("shows the team's pool and sends the team", async () => {
    renderForm();
    await screen.findByDisplayValue("Sales bot");
    await waitFor(() =>
      expect(screen.getByTestId("app-team-pool")).toHaveTextContent("Team pool: $40.00 of $100.00 unallocated"),
    );
    expect(screen.getByTestId("app-budget-help")).toHaveTextContent("Requests are refused once");

    fireEvent.click(screen.getByRole("button", { name: "Update app" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, body] = apiClient.patch.mock.calls[0];
    expect(body.data.attributes.team_id).toBe(3);
    expect(body.data.attributes.monthly_budget).toBe(20);
  });

  it("shows why the server refused an allocation", async () => {
    apiClient.patch.mockRejectedValue({
      response: { data: { errors: [{ detail: "allocation exceeds the team's unallocated budget: requested 90.00, 40.00 of the team budget is unallocated" }] } },
    });
    renderForm();
    await screen.findByDisplayValue("Sales bot");

    fireEvent.click(screen.getByRole("button", { name: "Update app" }));
    expect(await screen.findByText(/40.00 of the team budget is unallocated/)).toBeInTheDocument();
  });
});
