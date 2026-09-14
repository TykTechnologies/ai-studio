import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import CatalogueForm from "./CatalogueForm";
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
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({}),
}));

const apiClient = require("../../utils/apiClient").default;

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <CatalogueForm />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("CatalogueForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({
      data: { data: [{ id: "7", attributes: { name: "OpenAI", active: true } }] },
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "42" } } });
  });

  // The old defect: a "+" button was the actual commit, so picking a provider
  // and pressing "Create catalog" produced a catalog with zero members. The
  // RelationshipPicker has no pending step: choosing an option adds it, and
  // the form saves exactly what the chips show.
  it("saves a provider chosen in the picker with no extra add step", async () => {
    renderForm();

    await waitFor(() =>
      expect(screen.getByLabelText(/Catalog Name/)).toBeInTheDocument()
    );

    fireEvent.change(screen.getByLabelText(/Catalog Name/), {
      target: { value: "Support team" },
    });

    expect(screen.getByText("No LLM providers selected")).toBeInTheDocument();
    expect(screen.getByTestId("relationship-picker-caption")).toHaveTextContent(
      "Changes apply when you save this form."
    );

    // Pick a provider from the Autocomplete; it becomes a chip immediately.
    const addInput = screen.getByRole("combobox", { name: "Add LLM provider" });
    fireEvent.focus(addInput);
    fireEvent.mouseDown(addInput);
    fireEvent.click(await screen.findByRole("option", { name: "OpenAI" }));
    expect(screen.getByLabelText("Remove OpenAI")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /create catalog/i }));

    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith(
        "/catalogues/42/llms",
        { data: { id: "7", type: "LLM" } }
      );
    });
  });

  it("does not save a provider removed via its chip", async () => {
    renderForm();

    await waitFor(() =>
      expect(screen.getByLabelText(/Catalog Name/)).toBeInTheDocument()
    );
    fireEvent.change(screen.getByLabelText(/Catalog Name/), {
      target: { value: "Support team" },
    });

    const addInput = screen.getByRole("combobox", { name: "Add LLM provider" });
    fireEvent.focus(addInput);
    fireEvent.mouseDown(addInput);
    fireEvent.click(await screen.findByRole("option", { name: "OpenAI" }));
    fireEvent.click(screen.getByLabelText("Remove OpenAI"));
    expect(screen.getByText("No LLM providers selected")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /create catalog/i }));

    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith("/catalogues", expect.anything());
    });
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/catalogues/42/llms",
      expect.anything()
    );
  });

  describe("unsaved changes", () => {
    const DirtyProbe = () => {
      const { isDirty } = useUnsavedChanges();
      return <span data-testid="registry-dirty">{String(isDirty)}</span>;
    };

    const renderGuarded = () =>
      render(
        <ThemeProvider theme={testTheme}>
          <MemoryRouter>
            <UnsavedChangesProvider>
              <CatalogueForm />
              <DirtyProbe />
            </UnsavedChangesProvider>
          </MemoryRouter>
        </ThemeProvider>
      );

    const pickOpenAI = async () => {
      const addInput = screen.getByRole("combobox", { name: "Add LLM provider" });
      fireEvent.focus(addInput);
      fireEvent.mouseDown(addInput);
      fireEvent.click(await screen.findByRole("option", { name: "OpenAI" }));
    };

    it("Cancel returns to the list when nothing changed", async () => {
      renderGuarded();
      await screen.findByLabelText(/Catalog Name/);
      expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");

      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/llms");
      expect(screen.queryByTestId("unsaved-changes-dialog")).toBeNull();
    });

    it("a picker selection counts as a change and Cancel then prompts", async () => {
      renderGuarded();
      await screen.findByLabelText(/Catalog Name/);
      await pickOpenAI();
      expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(await screen.findByTestId("unsaved-changes-dialog")).toBeInTheDocument();
      expect(mockNavigate).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole("button", { name: "Stay" }));
      await waitFor(() => expect(screen.queryByTestId("unsaved-changes-dialog")).toBeNull());
      // Removing the chip again puts the form back on its baseline.
      fireEvent.click(screen.getByLabelText("Remove OpenAI"));
      expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
    });

    it("a successful save marks the form clean before redirecting", async () => {
      renderGuarded();
      await screen.findByLabelText(/Catalog Name/);
      fireEvent.change(screen.getByLabelText(/Catalog Name/), {
        target: { value: "Support team" },
      });
      await pickOpenAI();
      expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

      fireEvent.click(screen.getByRole("button", { name: /create catalog/i }));
      await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith("/admin/catalogs/llms", expect.anything()));
      // The form unmounts on redirect in the app; here navigate is a mock,
      // so the registry state after markSaved() is observable.
      await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));
    });
  });
});
