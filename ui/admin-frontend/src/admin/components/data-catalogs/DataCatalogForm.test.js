import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import DataCatalogForm from "./DataCatalogForm";
import {
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../../../components/unsaved-changes";

jest.mock("../../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});

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

const apiClient = require("../../utils/apiClient").default;

const DirtyProbe = () => {
  const { isDirty } = useUnsavedChanges();
  return <span data-testid="registry-dirty">{String(isDirty)}</span>;
};

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <UnsavedChangesProvider>
          <DataCatalogForm />
          <DirtyProbe />
        </UnsavedChangesProvider>
      </MemoryRouter>
    </ThemeProvider>
  );

const datasource = { id: "9", attributes: { name: "Docs index" } };
const tag = { id: "3", attributes: { name: "internal" } };

describe("DataCatalogForm unsaved changes", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    apiClient.get.mockImplementation((url) => {
      if (url === "/datasources") return Promise.resolve({ data: { data: [datasource] } });
      if (url === "/tags") return Promise.resolve({ data: { data: [tag] } });
      if (url === "/data-catalogues/5") {
        return Promise.resolve({
          data: {
            data: {
              attributes: {
                name: "Existing",
                short_description: "short",
                long_description: "",
                icon: "",
                datasources: [datasource],
                tags: [],
              },
            },
          },
        });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "42" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
  });

  it("has a Cancel button that returns to the list when clean", async () => {
    renderForm();
    await screen.findByLabelText(/Catalog Name/);
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/data");
  });

  it("loading an existing catalog with its members is not a change; a picker change is", async () => {
    mockParams = { id: "5" };
    renderForm();
    await screen.findByDisplayValue("Existing");
    expect(screen.getByLabelText("Remove Docs index")).toBeInTheDocument();
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.click(screen.getByLabelText("Remove Docs index"));
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByTestId("unsaved-changes-dialog")).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("a successful save marks the form clean before redirecting", async () => {
    renderForm();
    await screen.findByLabelText(/Catalog Name/);
    fireEvent.change(screen.getByLabelText(/Catalog Name/), { target: { value: "New" } });
    fireEvent.change(screen.getByLabelText(/Short Description/), { target: { value: "s" } });
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: /create catalog/i }));
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/data", expect.anything())
    );
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));
  });
});
