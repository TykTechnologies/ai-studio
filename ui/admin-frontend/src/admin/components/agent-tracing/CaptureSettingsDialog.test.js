import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import agentTracingService from "../../services/agentTracingService";
import CaptureSettingsDialog from "./CaptureSettingsDialog";

jest.mock("../../services/agentTracingService");

// CRA resets mock implementations between tests, so they are set here.
beforeEach(() => {
  agentTracingService.getConfig.mockResolvedValue({ content_ceiling: "redacted", content_default_mode: "metadata" });
});

const open = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <CaptureSettingsDialog open onClose={() => {}} />
    </ThemeProvider>,
  );

// jyx.172: the ceiling and default are environment settings in this
// release, shown read-only with what they mean and where they are set.
describe("CaptureSettingsDialog", () => {
  it("shows the ceiling and default, their meaning and their environment variables", async () => {
    open();
    expect(await screen.findByTestId("capture-content_ceiling")).toHaveTextContent("Redacted");
    expect(screen.getByTestId("capture-content_default_mode")).toHaveTextContent("Metadata");
    expect(screen.getByText(/The most any App, request header or default may capture/)).toBeInTheDocument();
    expect(screen.getByText(/What Apps without their own content mode capture/)).toBeInTheDocument();
    expect(screen.getByText("AGENT_TRACING_CONTENT_CEILING")).toBeInTheDocument();
    expect(screen.getByText("AGENT_TRACING_CONTENT_DEFAULT_MODE")).toBeInTheDocument();
    expect(screen.getByText(/The default is never above the ceiling/)).toBeInTheDocument();
  });

  it("offers no way to change them", async () => {
    open();
    await screen.findByTestId("capture-settings");
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(agentTracingService.updateCaptureSettings).toBeUndefined();
  });

  it("says when the settings cannot be loaded", async () => {
    agentTracingService.getConfig.mockRejectedValue(new Error("boom"));
    open();
    expect(await screen.findByText("The capture settings could not be loaded.")).toBeInTheDocument();
  });
});
