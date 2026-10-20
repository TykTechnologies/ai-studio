import React from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import useSystemFeatures from "../../hooks/useSystemFeatures";
import { usePermissions } from "../../context/PermissionsContext";
import {
  AppTracingSection,
  ResourceTracingSection,
  appTracingFromResponse,
  appTracingToRequest,
  validateAppTracing,
} from "./TracingSettingsSection";

jest.mock("../../hooks/useSystemFeatures");
jest.mock("../../context/PermissionsContext");

const wrap = (ui) => render(<ThemeProvider theme={testTheme}>{ui}</ThemeProvider>);

const setFeature = (on) => useSystemFeatures.mockReturnValue({ features: { feature_agent_tracing: on } });
const setPerms = (...perms) =>
  usePermissions.mockReturnValue({ can: (p) => perms.includes(p) || perms.includes("*") });

// CRA resets mock implementations between tests, so they are set here.
beforeEach(() => {
  setFeature(true);
  setPerms("*");
});

describe("visibility", () => {
  it("renders nothing when agent tracing is off (Community Edition, or not enabled)", () => {
    setFeature(false);
    const { container } = wrap(
      <>
        <ResourceTracingSection resourceLabel="tool" contentModeMax="" onContentModeMaxChange={() => {}} />
        <AppTracingSection value={appTracingFromResponse(null)} onChange={() => {}} />
      </>,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the section when agent tracing is on", () => {
    wrap(<ResourceTracingSection resourceLabel="tool" contentModeMax="" onContentModeMaxChange={() => {}} />);
    expect(screen.getByTestId("tracing-settings-section")).toBeInTheDocument();
    expect(screen.getByText("Tracing")).toBeInTheDocument();
  });
});

describe("ResourceTracingSection", () => {
  it("changes the content mode cap", () => {
    const onChange = jest.fn();
    wrap(<ResourceTracingSection resourceLabel="data source" contentModeMax="" onContentModeMaxChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Tracing" }));
    fireEvent.mouseDown(screen.getByRole("combobox", { name: /maximum content mode/i }));
    fireEvent.click(within(screen.getByRole("listbox")).getByText(/^Metadata/));
    expect(onChange).toHaveBeenCalledWith("metadata");
  });

  it("offers the propagation opt-out only where a handler is given (LLMs)", () => {
    const onOptOut = jest.fn();
    const { rerender } = wrap(
      <ResourceTracingSection resourceLabel="tool" contentModeMax="" onContentModeMaxChange={() => {}} />,
    );
    expect(screen.queryByLabelText(/don't send trace context/i)).not.toBeInTheDocument();
    rerender(
      <ThemeProvider theme={testTheme}>
        <ResourceTracingSection
          resourceLabel="LLM"
          contentModeMax=""
          onContentModeMaxChange={() => {}}
          propagationOptOut={false}
          onPropagationOptOutChange={onOptOut}
        />
      </ThemeProvider>,
    );
    fireEvent.click(screen.getByLabelText(/don't send trace context/i));
    expect(onOptOut).toHaveBeenCalledWith(true);
  });
});

describe("AppTracingSection", () => {
  it("edits a field and leaves the others alone", () => {
    const onChange = jest.fn();
    const value = appTracingFromResponse({ content_mode: "redacted" });
    wrap(<AppTracingSection value={value} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/session idle timeout/i), { target: { value: "2h" } });
    expect(onChange).toHaveBeenCalledWith({ ...value, session_idle_timeout: "2h" });
  });

  it("is read-only without traces:write, which every tracing change needs (jyx.99)", () => {
    setPerms("apps:write");
    const { unmount } = wrap(<AppTracingSection value={appTracingFromResponse(null)} onChange={() => {}} />);
    expect(screen.getByTestId("tracing-read-only")).toHaveTextContent(/traces:write/);
    expect(screen.getByLabelText(/let the app's owner view captured content/i)).toBeDisabled();
    expect(screen.getByLabelText(/session idle timeout/i)).toBeDisabled();
    expect(screen.getByLabelText(/keep captured content/i)).toBeDisabled();
    // The three selects: tracing on/off, content mode, request ceiling.
    const selects = screen.getAllByRole("combobox", { hidden: true });
    expect(selects).toHaveLength(3);
    for (const select of selects) expect(select).toHaveAttribute("aria-disabled", "true");
    unmount();

    setPerms("apps:write", "traces:write");
    const onChange = jest.fn();
    wrap(<AppTracingSection value={appTracingFromResponse(null)} onChange={onChange} />);
    const box = screen.getByLabelText(/let the app's owner view captured content/i);
    expect(box).toBeEnabled();
    fireEvent.click(box);
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ owner_may_view_content: true }));
  });

  it("resource settings are read-only without traces:write", () => {
    setPerms("llms:write");
    wrap(
      <ResourceTracingSection
        contentModeMax=""
        onContentModeMaxChange={() => {}}
        propagationOptOut={false}
        onPropagationOptOutChange={() => {}}
        resourceLabel="LLM"
      />
    );
    expect(screen.getByTestId("tracing-read-only")).toBeInTheDocument();
    expect(screen.getByLabelText(/don't send trace context/i)).toBeDisabled();
    expect(screen.getByRole("combobox", { hidden: true })).toHaveAttribute("aria-disabled", "true");
  });

  it("shows field errors", () => {
    wrap(
      <AppTracingSection
        value={appTracingFromResponse(null)}
        onChange={() => {}}
        errors={{ payload_days: "A whole number of days from 0 to 365" }}
      />,
    );
    expect(screen.getByText("A whole number of days from 0 to 365")).toBeInTheDocument();
  });
});

describe("App settings helpers", () => {
  it("round-trips the stored settings and omits fields left on inherit", () => {
    const stored = {
      enabled: false,
      content_mode: "hashed",
      max_request_override: "redacted",
      session_idle_timeout: "45m",
      payload_days: 0,
      owner_may_view_content: true,
    };
    expect(appTracingToRequest(appTracingFromResponse(stored))).toEqual(stored);
    expect(appTracingToRequest(appTracingFromResponse(null))).toEqual({});
    expect(appTracingToRequest(appTracingFromResponse({ enabled: true }))).toEqual({ enabled: true });
  });

  it("validates like the server", () => {
    const form = (overrides) => ({ ...appTracingFromResponse(null), ...overrides });
    expect(validateAppTracing(form({}))).toEqual({});
    expect(validateAppTracing(form({ session_idle_timeout: "1h30m", payload_days: "7" }))).toEqual({});
    expect(validateAppTracing(form({ session_idle_timeout: "168h" }))).toEqual({});
    expect(validateAppTracing(form({ session_idle_timeout: "soon" }))).toHaveProperty("session_idle_timeout");
    expect(validateAppTracing(form({ session_idle_timeout: "30s" }))).toHaveProperty("session_idle_timeout");
    expect(validateAppTracing(form({ session_idle_timeout: "169h" }))).toHaveProperty("session_idle_timeout");
    expect(validateAppTracing(form({ payload_days: "1.5" }))).toHaveProperty("payload_days");
    expect(validateAppTracing(form({ payload_days: "366" }))).toHaveProperty("payload_days");
    expect(validateAppTracing(form({ payload_days: "-1" }))).toHaveProperty("payload_days");
  });
});
