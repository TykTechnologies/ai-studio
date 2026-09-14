import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
import { useEdition } from "../../context/EditionContext";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: {} }),
}));
jest.mock("../../services/pluginService", () => ({
  __esModule: true,
  default: { listPlugins: jest.fn(), getAvailableHookTypes: jest.fn(), getHookTypeLabel: jest.fn() },
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({}),
}));

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <LLMForm />
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("LLMForm validation and layout", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: false });
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/llms") return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9" } } });
    apiClient.put.mockResolvedValue({ data: {} });
  });

  it("renders in-page errors on an empty submit instead of relying on the browser", async () => {
    renderForm();
    fireEvent.click(await screen.findByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(screen.getByTestId("llm-form-errors")).toBeInTheDocument());
    expect(screen.getByTestId("llm-form-errors")).toHaveTextContent("Name is required");
    expect(screen.getByTestId("llm-form-errors")).toHaveTextContent("Vendor is required");
    // Field-level helper text too.
    expect(screen.getAllByText("Name is required").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Vendor is required").length).toBeGreaterThan(0);
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("edits privacy as a named level plus score, and keeps the two in sync", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add LLM provider" });
    const score = screen.getByRole("spinbutton", { name: "Privacy level score" });
    const level = screen.getByTestId("privacy-level-select");
    // Starts at 0 = Public.
    expect(score).toHaveValue(0);
    expect(level).toHaveValue("public");
    expect(
      screen.getByText("LLM providers can only be used with tools and data sources at or below their level."),
    ).toBeInTheDocument();
    // Choosing Confidential sets the default score for the band.
    fireEvent.mouseDown(screen.getByRole("combobox", { name: /Privacy level/ }));
    fireEvent.click(screen.getByText("Confidential (51–75)"));
    expect(score).toHaveValue(75);
    // Typing a score moves the level.
    fireEvent.change(score, { target: { value: "10" } });
    expect(level).toHaveValue("public");
    // The number is what gets sent.
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Claude" } });
    fireEvent.mouseDown(screen.getByRole("combobox", { name: /Vendor/ }));
    fireEvent.click(await screen.findByRole("option", { name: /Anthropic/ }));
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/llms", expect.anything()));
    expect(apiClient.post.mock.calls.find((c) => c[0] === "/llms")[1].data.attributes.privacy_score).toBe(10);
  });

  it("shows Access Details expanded with the credential hint, and labels the live switch Active", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add LLM provider" });
    // Expanded by default: the fields are in the DOM without clicking the accordion.
    expect(screen.getByLabelText("API Endpoint")).toBeVisible();
    expect(screen.getByLabelText("API Key")).toBeVisible();
    expect(
      screen.getByText("Enter the key, or reference a stored secret as $SECRET/name or an environment variable as $ENV/NAME."),
    ).toBeInTheDocument();
    // The switch lives in the collapsed Portal Display accordion.
    fireEvent.click(screen.getByRole("button", { name: "Portal Display Information" }));
    expect(await screen.findByRole("checkbox", { name: "Active" })).toBeInTheDocument();
    expect(screen.getByText("Available to the portal and gateway when on")).toBeInTheDocument();
    expect(screen.queryByText("Enabled in Proxy")).not.toBeInTheDocument();
  });

  it("navigates straight to the list with a success toast after saving", async () => {
    renderForm();
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "My LLM" } });
    // Pick a vendor through the select.
    fireEvent.mouseDown(screen.getByLabelText(/Vendor/));
    const option = await screen.findAllByRole("option");
    fireEvent.click(option[0]);
    fireEvent.click(screen.getByRole("button", { name: "Add LLM provider" }));
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/admin/llms", {
        state: { snackbar: { message: "LLM provider created successfully", severity: "success" } },
      }),
    );
  });
});
