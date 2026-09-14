import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import ToolCatalogueForm from "./ToolCatalogueForm";
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
          <ToolCatalogueForm />
          <DirtyProbe />
        </UnsavedChangesProvider>
      </MemoryRouter>
    </ThemeProvider>
  );

const tool = { id: "11", attributes: { name: "Weather" } };
const tag = { id: "3", attributes: { name: "internal" } };

describe("ToolCatalogueForm unsaved changes", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    jest.spyOn(console, "log").mockImplementation(() => {});
    apiClient.get.mockImplementation((url) => {
      if (url === "/tools") return Promise.resolve({ data: { data: [tool] } });
      if (url === "/tags") return Promise.resolve({ data: { data: [tag] } });
      if (url === "/tool-catalogues/8") {
        return Promise.resolve({
          data: {
            data: {
              attributes: {
                name: "Existing tools",
                short_description: "",
                long_description: "",
                icon: "",
                tools: [tool],
                tags: [tag],
              },
            },
          },
        });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "42" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "8" } } });
  });

  afterEach(() => {
    console.log.mockRestore();
  });

  it("Cancel and the back link return to the list when clean", async () => {
    renderForm();
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/tools");
    mockNavigate.mockClear();

    fireEvent.click(screen.getByRole("button", { name: /Back to catalogs/ }));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/tools");
  });

  it("loading an existing catalog is not a change; the back link prompts once edited", async () => {
    mockParams = { id: "8" };
    renderForm();
    await screen.findByDisplayValue("Existing tools");
    await screen.findByLabelText("Remove Weather");
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Renamed" } });
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: /Back to catalogs/ }));
    expect(await screen.findByTestId("unsaved-changes-dialog")).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Leave without saving" }));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/tools"));
  });

  it("a successful save marks the form clean before redirecting", async () => {
    renderForm();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "New" } });
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: /create catalog/i }));
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/tools", expect.anything())
    );
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));
  });
});
