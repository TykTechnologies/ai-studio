import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import testTheme from "../utils/testTheme";
import AgentSessions from "./AgentSessions";
import agentTracingService from "../services/agentTracingService";
import { useEdition } from "../context/EditionContext";
import useSystemFeatures from "../hooks/useSystemFeatures";
import { listAll } from "../utils/listAll";
import { PermissionDeniedError } from "../utils/apiErrors";
import PermissionsContext from "../context/PermissionsContext";

jest.mock("../services/agentTracingService", () => ({
  __esModule: true,
  default: { listSessions: jest.fn(), getConfig: jest.fn() },
}));
jest.mock("../context/EditionContext");
jest.mock("../hooks/useSystemFeatures");
jest.mock("../utils/listAll", () => ({ __esModule: true, listAll: jest.fn() }));
jest.mock("../../components/common/Icon", () => () => null);

const UUID = "0192a4c8-7f00-7000-8000-000000000001";

const session = (over = {}) => ({
  id: UUID,
  app_id: 7,
  app_name: "Support bot",
  user_id: 3,
  user_name: "Ada",
  source: "declared",
  name: "refund flow",
  status: "closed",
  started_at: "2026-10-20T10:00:00Z",
  last_activity_at: "2026-10-20T10:01:00Z",
  totals: { turn_count: 2, span_count: 9, llm_call_count: 3, tool_call_count: 4, error_count: 1, input_tokens: 100, output_tokens: 50, cost_micros: 1234567, currency: "USD", dropped_count: 0 },
  duration_ms: 60000,
  gateway_event_count: 1,
  agent_names: ["planner", "coder", "tester"],
  models: ["gpt-4.1"],
  tool_names: ["search"],
  confidence_counts: { declared: 1, exact: 2, strong: 1, weak: 0 },
  content_mode: "metadata",
  sampled: true,
  ...over,
});

let lastLocation;
const LocationProbe = () => {
  lastLocation = useLocation();
  return null;
};

const renderPage = (initial = "/admin/agent-sessions") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[initial]}>
        <Routes>
          <Route
            path="/admin/agent-sessions"
            element={
              <>
                <AgentSessions />
                <LocationProbe />
              </>
            }
          />
          <Route path="/admin/agent-sessions/:id" element={<div data-testid="detail-page" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

const lastBody = () => {
  const calls = agentTracingService.listSessions.mock.calls;
  return calls[calls.length - 1][0];
};

// CRA resets mock implementations before each test, so they are set here.
beforeEach(() => {
  lastLocation = null;
  useEdition.mockReturnValue({ isEnterprise: true });
  useSystemFeatures.mockReturnValue({ features: { feature_agent_tracing: true }, loading: false });
  listAll.mockResolvedValue({ data: { data: [{ id: 7, attributes: { name: "Support bot" } }] } });
  agentTracingService.listSessions.mockResolvedValue({ data: [session()], next_cursor: null });
});

describe("AgentSessions", () => {
  it("shows the Enterprise panel in the Community Edition without calling the API", () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    renderPage();
    expect(screen.getByText("Agent Call Tracing")).toBeInTheDocument();
    expect(agentTracingService.listSessions).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Capture settings" })).toBeNull();
  });

  // jyx.172: the install-wide capture settings open from the page header.
  it("opens the capture settings from the header", async () => {
    agentTracingService.getConfig.mockResolvedValue({ content_ceiling: "full", content_default_mode: "off" });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Capture settings" }));
    expect(await screen.findByRole("dialog", { name: "Content capture" })).toBeInTheDocument();
    expect(agentTracingService.getConfig).toHaveBeenCalled();
  });

  it("says tracing is off when the feature is not running, without calling the API", () => {
    useSystemFeatures.mockReturnValue({ features: { feature_agent_tracing: false }, loading: false });
    renderPage();
    expect(screen.getByTestId("tracing-disabled")).toBeInTheDocument();
    expect(agentTracingService.listSessions).not.toHaveBeenCalled();
  });

  it("shows the permission panel when the API denies traces:read", async () => {
    agentTracingService.listSessions.mockRejectedValue(new PermissionDeniedError("denied", "traces:read"));
    renderPage();
    expect(await screen.findByTestId("permission-denied-panel")).toBeInTheDocument();
  });

  it("renders the §6.1 columns, the inferred chip and the live dot", async () => {
    agentTracingService.listSessions.mockResolvedValue({
      data: [session(), session({ id: "0192a4c8-7f00-7000-8000-000000000002", name: "", source: "inferred", status: "active" })],
      next_cursor: null,
    });
    renderPage();
    expect(await screen.findByText("refund flow")).toBeInTheDocument();
    expect(screen.getByText("0192a4c8", { selector: "p" })).toBeInTheDocument();
    expect(screen.getAllByTestId("inferred-chip")).toHaveLength(1);
    expect(screen.getAllByTestId("live-dot")).toHaveLength(1);
    expect(screen.getAllByText("Support bot").length).toBeGreaterThan(0);
    expect(screen.getAllByText("3 · planner, coder +1")).toHaveLength(2);
    expect(screen.getAllByText("1 min 0 s")).toHaveLength(2);
    expect(screen.getAllByTestId("correlation-quality-bar")[0]).toHaveAttribute(
      "aria-label",
      "Correlation: Declared 1, Exact 2, Inferred 1",
    );
  });

  it("labels a pseudonymised end user for a viewer without trace-content:read", async () => {
    agentTracingService.listSessions.mockResolvedValue({
      data: [session({ user_name: "", user_id: null, end_user: "0123456789abcdef0123456789abcdef" })],
      next_cursor: null,
    });
    renderPage();
    expect(await screen.findByTestId("end-user-pseudonym")).toHaveTextContent("0123456789abcdef0123456789abcdef (pseudonym)");
  });

  it("shows the end user unlabelled to a viewer with trace-content:read", async () => {
    agentTracingService.listSessions.mockResolvedValue({
      data: [session({ user_name: "", user_id: null, end_user: "alice@example.com" })],
      next_cursor: null,
    });
    render(
      <PermissionsContext.Provider value={{ can: (perm) => perm === "trace-content:read" }}>
        <ThemeProvider theme={testTheme}>
          <MemoryRouter initialEntries={["/admin/agent-sessions"]}>
            <AgentSessions />
          </MemoryRouter>
        </ThemeProvider>
      </PermissionsContext.Provider>,
    );
    expect(await screen.findByText("alice@example.com")).toBeInTheDocument();
    expect(screen.queryByTestId("end-user-pseudonym")).not.toBeInTheDocument();
  });

  it("warns when sessions are incomplete because captures were dropped", async () => {
    agentTracingService.listSessions.mockResolvedValue({
      data: [session({ totals: { ...session().totals, dropped_count: 4 } })],
      next_cursor: null,
    });
    renderPage();
    expect(await screen.findByTestId("dropped-banner")).toHaveTextContent("One session on this page is incomplete");
  });

  it("reads URL filters into the query and writes filter changes back to the URL", async () => {
    renderPage(`/admin/agent-sessions?status=error&app=7&sort=-cost&session=${UUID}`);
    await waitFor(() => expect(agentTracingService.listSessions).toHaveBeenCalled());
    expect(lastBody()).toMatchObject({ status: "error", app_id: 7, sort: "-cost", session_id: UUID });

    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Has errors" }));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Yes"));
    await waitFor(() => expect(lastBody()).toMatchObject({ has_errors: true, status: "error" }));
    expect(new URLSearchParams(lastLocation.search).get("errors")).toBe("yes");
  });

  it("hints under the id filters that results show pseudonyms, only without trace-content:read", async () => {
    const hint = "Matches the exact id sent by the client; results show pseudonyms.";
    const { unmount } = renderPage();
    await screen.findByText("refund flow");
    fireEvent.click(screen.getByText("More filters"));
    expect(screen.getAllByText(hint)).toHaveLength(2);
    unmount();

    render(
      <PermissionsContext.Provider value={{ can: (perm) => perm === "trace-content:read" }}>
        <ThemeProvider theme={testTheme}>
          <MemoryRouter initialEntries={["/admin/agent-sessions"]}>
            <AgentSessions />
          </MemoryRouter>
        </ThemeProvider>
      </PermissionsContext.Provider>,
    );
    await screen.findByText("refund flow");
    fireEvent.click(screen.getByText("More filters"));
    expect(screen.queryByText(hint)).not.toBeInTheDocument();
    expect(screen.getByText("The user id the client declared")).toBeInTheDocument();
    expect(screen.getByText("Exact match")).toBeInTheDocument();
  });

  it("keeps name, end-user and external-id filters out of the URL", async () => {
    renderPage();
    await screen.findByText("refund flow");
    fireEvent.click(screen.getByText("More filters"));
    fireEvent.change(screen.getByLabelText("Agent name"), { target: { value: "planner" } });
    fireEvent.change(screen.getByLabelText(/^End user/), { target: { value: "alice@example.com" } });
    fireEvent.change(screen.getByLabelText(/Session or external id/), { target: { value: "conv-77" } });
    fireEvent.click(screen.getByRole("button", { name: "Find" }));
    await waitFor(() => expect(lastBody()).toMatchObject({ agent: "planner", end_user: "alice@example.com", external_id: "conv-77" }));
    expect(lastLocation.search).not.toMatch(/planner|alice|conv-77/);

    // A session UUID, by contrast, is a URL filter.
    fireEvent.change(screen.getByLabelText(/Session or external id/), { target: { value: UUID } });
    fireEvent.click(screen.getByRole("button", { name: "Find" }));
    await waitFor(() => expect(lastBody()).toMatchObject({ session_id: UUID }));
    expect(lastBody().external_id).toBeUndefined();
    expect(new URLSearchParams(lastLocation.search).get("session")).toBe(UUID);
  });

  it("pages forward and back with the cursor", async () => {
    agentTracingService.listSessions.mockImplementation((body) =>
      Promise.resolve(
        body.cursor === "c2"
          ? { data: [session({ name: "second page" })], next_cursor: null }
          : { data: [session()], next_cursor: "c2" },
      ),
    );
    renderPage();
    await screen.findByText("refund flow");
    expect(lastBody().cursor).toBeUndefined();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("second page")).toBeInTheDocument();
    expect(lastBody().cursor).toBe("c2");
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(await screen.findByText("refund flow")).toBeInTheDocument();
    expect(lastBody().cursor).toBeUndefined();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
  });

  it("goes back to the first page when a filter changes", async () => {
    agentTracingService.listSessions.mockResolvedValue({ data: [session()], next_cursor: "c2" });
    renderPage();
    await screen.findByText("refund flow");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(lastBody().cursor).toBe("c2"));
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Status" }));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Active"));
    await waitFor(() => expect(lastBody()).toMatchObject({ status: "active" }));
    expect(lastBody().cursor).toBeUndefined();
    const withStatus = agentTracingService.listSessions.mock.calls.filter(([b]) => b.status === "active");
    expect(withStatus.every(([b]) => b.cursor === undefined)).toBe(true);
  });

  it("opens the session on row click", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("refund flow"));
    expect(await screen.findByTestId("detail-page")).toBeInTheDocument();
  });
});
