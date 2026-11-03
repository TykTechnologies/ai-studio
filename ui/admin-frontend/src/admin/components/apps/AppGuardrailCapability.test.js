import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import AppGuardrailCapability from "./AppGuardrailCapability";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), put: jest.fn() },
}));
let mockFullAdmin = true;
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: () => ({ isFullAdmin: mockFullAdmin, can: () => true }),
}));

const apiClient = require("../../utils/apiClient").default;

const unattested = {
  value: "",
  effective: "acting",
  suggestion: "read_only",
  reasons: ["No tools, datasources or MCP servers attached"],
};

const renderIt = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <AppGuardrailCapability appId="7" />
    </ThemeProvider>
  );

describe("AppGuardrailCapability", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockFullAdmin = true;
    apiClient.get.mockResolvedValue({ data: { data: unattested } });
    apiClient.put.mockImplementation((url, body) =>
      Promise.resolve({ data: { data: { ...unattested, value: body.value, effective: body.value || "acting" } } })
    );
  });

  it("shows an unattested app as treated as acting, with the suggestion", async () => {
    renderIt();
    expect(await screen.findByText("Not attested (treated as acting)")).toBeInTheDocument();
    expect(screen.getByText(/Suggested: Read-only/)).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/apps/7/guardrail-capability");
  });

  it("lets a full admin accept the suggestion in one click", async () => {
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: "Accept suggestion: Read-only" }));
    await waitFor(() =>
      expect(apiClient.put).toHaveBeenCalledWith("/apps/7/guardrail-capability", { value: "read_only" })
    );
    expect(await screen.findByTestId("capability-chip")).toHaveTextContent("Read-only");
    expect(screen.queryByRole("button", { name: /Accept suggestion/ })).not.toBeInTheDocument();
  });

  it("opens Change with the suggestion pre-selected and its reasons", async () => {
    apiClient.get.mockResolvedValue({
      data: {
        data: {
          value: "",
          effective: "acting",
          suggestion: "acting",
          reasons: ['Tool "GitHub": operation postComment (POST) is not read-only'],
        },
      },
    });
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: "Change" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByTestId("capability-reasons")).toHaveTextContent("postComment");
    expect(within(dialog).getByRole("radio", { name: /Acting/ })).toBeChecked();

    fireEvent.click(within(dialog).getByRole("radio", { name: /Reads with tools/ }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(apiClient.put).toHaveBeenCalledWith("/apps/7/guardrail-capability", { value: "read_tools" })
    );
  });

  it("is read-only for users who are not full admins", async () => {
    mockFullAdmin = false;
    renderIt();
    expect(await screen.findByText("Not attested (treated as acting)")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Change" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Accept suggestion/ })).not.toBeInTheDocument();
  });
});
