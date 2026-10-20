import React from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import testTheme from "../utils/testTheme";
import AgentSessionDetail from "./AgentSessionDetail";
import agentTracingService from "../services/agentTracingService";
import { useEdition } from "../context/EditionContext";
import { SESSION_ID, detail, ids, spans } from "../components/agent-tracing/testFixture";

jest.mock("../services/agentTracingService", () => ({
  __esModule: true,
  default: {
    getSession: jest.fn(),
    listSpans: jest.fn(),
    getSpan: jest.fn(),
    getPayload: jest.fn(),
    search: jest.fn(),
    exportSession: jest.fn(),
  },
}));
jest.mock("../context/EditionContext");
jest.mock("../../components/common/Icon", () => () => null);

const renderPage = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[`/admin/agent-sessions/${SESSION_ID}`]}>
        <Routes>
          <Route path="/admin/agent-sessions/:sessionId" element={<AgentSessionDetail />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

const spanById = (id) => spans.find((s) => s.span_id === id);
const row = (id) => screen.queryAllByTestId("span-row").find((r) => r.getAttribute("data-span-id") === id);

// CRA resets mock implementations before each test, so they are set here.
beforeEach(() => {
  useEdition.mockReturnValue({ isEnterprise: true });
  agentTracingService.getSession.mockResolvedValue(detail());
  // Two skeleton pages, as the API pages spans by 1,000.
  agentTracingService.listSpans.mockImplementation((_id, params) =>
    Promise.resolve(
      params.cursor === "p2"
        ? { data: spans.slice(3), next_cursor: null }
        : { data: spans.slice(0, 3), next_cursor: "p2", resources: [{ type: "llm", id: "5", name: "GPT Production" }] },
    ),
  );
  agentTracingService.getSpan.mockImplementation((id) => Promise.resolve({ span: spanById(id), events: [], resources: [] }));
  agentTracingService.getPayload.mockResolvedValue({
    part: "request",
    content_state: "full",
    content: '{"model":"gpt-4.1","user":"[REDACTED:EMAIL]"}',
    media_type: "application/json",
    original_bytes: 900,
    returned_bytes: 900,
    truncated: false,
  });
  agentTracingService.search.mockResolvedValue({ matches: [{ span_id: ids.tool1, field: "tool_name", snippet: "web_search" }], total: 1, truncated: false });
});

// These tests drive the shell through the plain span list, which is view 2
// until the Timeline (jyx.31) replaces it; the Transcript has its own tests.
const loaded = async () => {
  await screen.findByText("refund flow");
  await screen.findByTestId("transcript");
  fireEvent.keyDown(document.body, { key: "2" });
  await waitFor(() => expect(row(ids.client)).toBeInTheDocument());
};

describe("AgentSessionDetail", () => {
  it("shows the Enterprise panel in the Community Edition without calling the API", () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    renderPage();
    expect(screen.getByText("Agent Call Tracing")).toBeInTheDocument();
    expect(agentTracingService.getSession).not.toHaveBeenCalled();
  });

  it("renders the header, summary and timeline strips and loads every span page", async () => {
    renderPage();
    await loaded();
    expect(agentTracingService.listSpans).toHaveBeenCalledTimes(2);
    // A content reader asks for inline previews; the server decides.
    expect(agentTracingService.listSpans.mock.calls[0][1]).toEqual({ limit: 1000, include: "previews" });
    expect(agentTracingService.listSpans.mock.calls[1][1]).toEqual({ limit: 1000, include: "previews", cursor: "p2" });
    expect(screen.getByTestId("session-header")).toHaveTextContent("Support bot");
    expect(screen.getByTestId("summary-strip")).toHaveTextContent("web_search");
    expect(screen.getByTestId("timeline-strip")).toBeInTheDocument();
    expect(document.title).not.toContain("refund flow");
  });

  it("shares one selection: a click in the list selects in the timeline and opens the drawer", async () => {
    renderPage();
    await loaded();
    expect(agentTracingService.getSpan).not.toHaveBeenCalled();
    fireEvent.click(row(ids.tool1));
    const drawer = await screen.findByRole("dialog");
    expect(within(drawer).getByRole("heading", { name: "tools/call web_search" })).toBeInTheDocument();
    expect(row(ids.tool1)).toHaveAttribute("aria-selected", "true");
    expect(screen.getByTestId("selected-marker")).toBeInTheDocument();
    expect(agentTracingService.getSpan).toHaveBeenCalledWith(ids.tool1, 7);
  });

  it("loads content only when a content tab is shown, and renders redaction chips", async () => {
    renderPage();
    await loaded();
    fireEvent.click(row(ids.llm1));
    const drawer = await screen.findByRole("dialog");
    expect(agentTracingService.getPayload).not.toHaveBeenCalled();
    fireEvent.click(within(drawer).getByRole("tab", { name: "Input" }));
    await waitFor(() => expect(agentTracingService.getPayload).toHaveBeenCalledWith(ids.llm1, 7, "request", 2048));
    expect(await within(drawer).findByTestId("kv-table")).toBeInTheDocument();
    expect(within(drawer).getByText("REDACTED:EMAIL")).toBeInTheDocument();
    fireEvent.click(within(drawer).getByRole("button", { name: "JSON" }));
    expect(within(drawer).getByTestId("raw-view")).toHaveTextContent('"model":"gpt-4.1"');
  });

  it("shows placeholders and requests no content for a viewer without content access", async () => {
    agentTracingService.getSession.mockResolvedValue(detail({ can_read_content: false, can_export: false }));
    renderPage();
    await loaded();
    fireEvent.click(row(ids.llm1));
    const drawer = await screen.findByRole("dialog");
    fireEvent.click(within(drawer).getByRole("tab", { name: "Input" }));
    expect(within(drawer).getByText("Captured (900 B)")).toBeInTheDocument();
    expect(within(drawer).getByTestId("no-content-permission")).toBeInTheDocument();
    fireEvent.click(within(drawer).getByRole("tab", { name: "Output" }));
    expect(within(drawer).getByText("Redacted (2 values)")).toBeInTheDocument();
    expect(agentTracingService.getPayload).not.toHaveBeenCalled();
    // The modal drawer hides the page from the accessibility tree.
    expect(screen.getByRole("button", { name: "Export", hidden: true })).toBeDisabled();
  });

  it("shows the content-state placeholders that never load content", async () => {
    renderPage();
    await loaded();
    fireEvent.click(row(ids.tool1));
    let drawer = await screen.findByRole("dialog");
    fireEvent.click(within(drawer).getByRole("tab", { name: "Input" }));
    expect(within(drawer).getByText("Hashed: only a fingerprint was stored")).toBeInTheDocument();
    fireEvent.keyDown(drawer, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    fireEvent.click(row(ids.llm2));
    drawer = await screen.findByRole("dialog");
    fireEvent.click(within(drawer).getByRole("tab", { name: "Input" }));
    expect(within(drawer).getByText(/Content not captured \(mode: metadata/)).toBeInTheDocument();
    expect(agentTracingService.getPayload).not.toHaveBeenCalled();
  });

  it("works from the keyboard alone: focus trap in the drawer and focus returned on Esc", async () => {
    renderPage();
    await loaded();
    const grid = screen.getByRole("grid", { name: "Session spans" });
    act(() => grid.focus());
    fireEvent.keyDown(grid, { key: "j" });
    await waitFor(() => expect(row(ids.llm1)).toHaveAttribute("aria-selected", "true"));
    fireEvent.keyDown(grid, { key: "Enter" });
    // The modal drawer takes focus, and gives it back when it closes.
    const drawer = await screen.findByRole("dialog");
    await waitFor(() => expect(drawer).toHaveFocus());
    fireEvent.keyDown(drawer, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(grid).toHaveFocus());
  });

  it("finds in the session with the term in the request body and selects the match", async () => {
    renderPage();
    await loaded();
    fireEvent.keyDown(document.body, { key: "/" });
    const input = await screen.findByLabelText("Find in session");
    fireEvent.change(input, { target: { value: "web" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(agentTracingService.search).toHaveBeenCalledWith(SESSION_ID, "web"));
    expect(await screen.findByTestId("find-count")).toHaveTextContent("1 of 1");
    expect(row(ids.tool1)).toHaveAttribute("aria-selected", "true");
  });
});
