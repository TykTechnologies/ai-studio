import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import ModelRouterForm from "./ModelRouterForm";
import { useEdition } from "../../context/EditionContext";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
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

const catalogues = [
  { id: "2", attributes: { name: "Platform" } },
  { id: "1", attributes: { name: "Default" } },
];

const existingRouter = {
  data: {
    data: {
      id: "3",
      attributes: {
        name: "Prod",
        slug: "prod",
        description: "",
        short_description: "Routes GPT traffic",
        long_description: "",
        logo_url: "",
        api_compat: "openai",
        active: true,
        namespace: "",
        catalogues: [{ id: 1, name: "Default" }],
        pools: [
          {
            name: "gpt",
            model_pattern: "gpt-4o",
            selection_algorithm: "round_robin",
            priority: 0,
            vendors: [{ llm_id: 1, llm_slug: "openai", weight: 1, is_active: true, mappings: [] }],
          },
        ],
      },
    },
  },
};

const identity = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: ["model-routers:write", "model-routers:publish"],
  },
};

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity}>
          <ModelRouterForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const cataloguePicker = () =>
  screen.getAllByTestId("relationship-picker").find((el) => el.dataset.itemLabel === "catalog");

const pickerItems = () =>
  within(cataloguePicker())
    .queryAllByTestId("relationship-picker-item")
    .map((el) => el.textContent);

describe("ModelRouterForm portal fields and catalogues", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms") return Promise.resolve({ data: { data: [{ id: 1, attributes: { name: "OpenAI", slug: "openai" } }] } });
      if (url === "/catalogues") return Promise.resolve({ data: { data: catalogues }, headers: {} });
      if (url === "/model-routers/3") return Promise.resolve(existingRouter);
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9", attributes: {} } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "3", attributes: {} } } });
    apiClient.put.mockResolvedValue({ data: { data: { catalogues: [] } } });
  });

  it("creates the router with its portal fields, then publishes it in the chosen catalogs", async () => {
    renderForm();
    await waitFor(() => expect(within(cataloguePicker()).getByTestId("relationship-picker-options-count")).toHaveTextContent("2"));
    // Catalogues are listed through listAll (paged), not a bare page of 10.
    expect(apiClient.get).toHaveBeenCalledWith("/catalogues", { params: { page: 1, page_size: 100 } });

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Prod" } });
    fireEvent.change(screen.getByLabelText("Short Description"), { target: { value: "Routes GPT traffic" } });
    fireEvent.change(screen.getByLabelText("Logo URL"), { target: { value: "https://example.com/logo.png" } });
    fireEvent.click(within(cataloguePicker()).getByTestId("relationship-picker-add"));
    expect(pickerItems()).toEqual(["Platform"]);

    fireEvent.click(screen.getByRole("button", { name: "Add Pool" }));
    fireEvent.change(screen.getByLabelText(/^Pool Name/), { target: { value: "all" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Vendor" }));
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(apiClient.put).toHaveBeenCalled());
    const [, body] = apiClient.post.mock.calls[0];
    expect(apiClient.post.mock.calls[0][0]).toBe("/model-routers");
    expect(body.data.attributes).toEqual(
      expect.objectContaining({
        name: "Prod",
        slug: "prod",
        short_description: "Routes GPT traffic",
        long_description: "",
        logo_url: "https://example.com/logo.png",
      }),
    );
    expect(apiClient.put).toHaveBeenCalledWith("/model-routers/9/catalogues", { catalogue_ids: [2] });
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/model-routers", {
        state: { snackbar: { message: "Model Router created successfully", severity: "success" } },
      }),
    );
  });

  it("does not rewrite the catalogs on update when the selection is unchanged", async () => {
    mockParams = { id: "3" };
    renderForm();
    await screen.findByDisplayValue("Routes GPT traffic");
    await waitFor(() => expect(pickerItems()).toEqual(["Default"]));

    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(apiClient.patch.mock.calls[0][1].data.attributes.short_description).toBe("Routes GPT traffic");
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    expect(apiClient.put).not.toHaveBeenCalled();
  });

  it("replaces the catalog set on update when it changed, and warns if that fails", async () => {
    mockParams = { id: "3" };
    apiClient.put.mockRejectedValue(new Error("forbidden"));
    renderForm();
    await waitFor(() => expect(pickerItems()).toEqual(["Default"]));

    fireEvent.click(within(cataloguePicker()).getByTestId("relationship-picker-remove"));
    fireEvent.click(screen.getByRole("button", { name: "Update" }));

    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/model-routers/3/catalogues", { catalogue_ids: [] }));
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/model-routers", {
        state: { snackbar: { message: "Model Router saved, but publishing it in the catalogs failed", severity: "warning" } },
      }),
    );
  });
});
