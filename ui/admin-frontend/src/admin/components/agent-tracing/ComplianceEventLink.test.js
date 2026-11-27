import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import { usePermissions } from "../../context/PermissionsContext";
import { SessionDetailProvider } from "./SessionDetailContext";
import ComplianceEventLink, { requestIdOf } from "./ComplianceEventLink";
import { indexSpans } from "./spanModel";

jest.mock("../../utils/apiClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("../../context/PermissionsContext", () => ({ usePermissions: jest.fn() }));

// tkg.2: a guardrail event on the trace leads to its compliance events.
const REQ = "tyai_0123456789abcdef0123456789abcdef";
const llm = {
  span_id: "0000000000000001", trace_id: "t1", kind: "llm_inference", name: "chat", status: "error",
  start_time: "2026-11-27T10:00:00Z", end_time: "2026-11-27T10:00:01Z", attributes: { "tyk.request.id": REQ },
};
const guard = {
  span_id: "0000000000000002", trace_id: "t1", parent_span_id: llm.span_id, kind: "gateway_event", name: "guardrail",
  status: "error", start_time: "2026-11-27T10:00:00.5Z", end_time: "2026-11-27T10:00:00.5Z",
  gateway_event: { type: "guardrail", outcome: "denied", name: "no secrets" },
};
const detail = { session: { id: "s1", app_id: 7 }, agents: [], traces: [], viewer: { can_read_content: true } };

const Where = () => {
  const loc = useLocation();
  return <span data-testid="location">{loc.pathname + loc.search}</span>;
};

const renderLink = (span = guard, spans = [llm, guard]) =>
  render(
    <MemoryRouter initialEntries={["/admin/agent-sessions/s1"]}>
      <SessionDetailProvider detail={detail} spans={spans} resources={[]} previews="included">
        <Routes>
          <Route path="/admin/agent-sessions/s1" element={<ComplianceEventLink span={span} />} />
          <Route path="/admin/compliance" element={<Where />} />
        </Routes>
      </SessionDetailProvider>
    </MemoryRouter>,
  );

describe("ComplianceEventLink", () => {
  beforeEach(() => {
    usePermissions.mockReturnValue({ can: (p) => p === "compliance:read" });
    apiClient.get.mockResolvedValue({ data: { events: [] } });
  });

  it("reads the request id from the event's LLM span, for guardrail and filter events only", () => {
    const index = indexSpans([llm, guard]);
    expect(requestIdOf(guard, index)).toBe(REQ);
    expect(requestIdOf({ ...guard, gateway_event: { type: "budget" } }, index)).toBe("");
    expect(requestIdOf(guard, indexSpans([{ ...llm, attributes: { "tyk.request.id": "<b>x</b>" } }, guard]))).toBe("");
  });

  it("opens the one compliance event it finds", async () => {
    apiClient.get.mockResolvedValue({ data: { events: [{ id: 42, event_type: "guardrail.secrets.aws" }] } });
    renderLink();
    fireEvent.click(screen.getByTestId("compliance-event-link"));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/admin/compliance?event=42"));
    expect(apiClient.get).toHaveBeenCalledWith("/compliance/events-by-request", {
      params: { request_id: REQ, app_id: 7, at: guard.start_time },
    });
  });

  it("lists several", async () => {
    apiClient.get.mockResolvedValue({ data: { events: [{ id: 42, event_type: "guardrail.secrets.aws" }, { id: 43, event_type: "guardrail.judge" }] } });
    renderLink();
    fireEvent.click(screen.getByTestId("compliance-event-link"));
    const items = await screen.findAllByTestId("compliance-event-item");
    expect(items.map((a) => a.getAttribute("href"))).toEqual(["/admin/compliance?event=42", "/admin/compliance?event=43"]);
  });

  it("says when there is none", async () => {
    renderLink();
    fireEvent.click(screen.getByTestId("compliance-event-link"));
    expect(await screen.findByTestId("compliance-event-none")).toHaveTextContent(REQ);
  });

  it("falls back to the request id where compliance events are not available (403)", async () => {
    apiClient.get.mockRejectedValue({ response: { status: 403 } });
    renderLink();
    fireEvent.click(screen.getByTestId("compliance-event-link"));
    expect(await screen.findByTestId("request-id")).toHaveTextContent(REQ);
    expect(screen.queryByTestId("compliance-event-link")).not.toBeInTheDocument();
  });

  it("shows only the request id to a viewer without compliance read", () => {
    usePermissions.mockReturnValue({ can: () => false });
    renderLink();
    expect(screen.getByTestId("request-id")).toHaveTextContent(REQ);
    expect(screen.queryByTestId("compliance-event-link")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalled();
  });

  it("renders nothing without a request id", () => {
    const { container } = renderLink(guard, [{ ...llm, attributes: {} }, guard]);
    expect(container).toBeEmptyDOMElement();
  });
});
