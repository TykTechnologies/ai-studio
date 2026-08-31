import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import CatalogueForm from "./CatalogueForm";

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

  // The defect: the + button was the actual commit. Picking a provider and
  // pressing "Create catalog" produced a catalog with zero members, with no
  // error and the select still showing the choice. The only symptom was a
  // developer's empty portal several steps later.
  it("saves a provider that was selected but never added with +", async () => {
    renderForm();

    await waitFor(() =>
      expect(screen.getByLabelText(/Catalog Name/)).toBeInTheDocument()
    );

    fireEvent.change(screen.getByLabelText(/Catalog Name/), {
      target: { value: "Support team" },
    });

    // Pick a provider from the select, and deliberately do NOT press +.
    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "OpenAI" }));

    fireEvent.click(screen.getByRole("button", { name: /create catalog/i }));

    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith(
        "/catalogues/42/llms",
        { data: { id: "7", type: "LLM" } }
      );
    });
  });
});
