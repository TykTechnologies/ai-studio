import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import AppGuardrailBlocks from "./AppGuardrailBlocks";

const client = (rows, fail = false) => ({
  get: jest.fn(() => (fail ? Promise.reject(new Error("x")) : Promise.resolve({ data: { data: rows } }))),
});

describe("AppGuardrailBlocks", () => {
  it("lists the app's guardrail blocks with the reason and request id", async () => {
    const c = client([
      {
        timestamp: "2026-10-21T10:05:47Z",
        filter: "Wolf Defender: stop conversation",
        action: "block",
        reason_code: "guardrail_too_large_to_scan",
        reason: "a tool result from fetch_results of 321,097 characters is longer than the guardrail scans (262,144 characters), so it was not checked",
        request_id: "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b",
      },
      { timestamp: "2026-10-21T10:13:33Z", filter: "Wolf Defender: quarantine", action: "remove", reason: "the guardrail found a likely prompt injection in the definition of the tool x" },
    ]);
    render(<AppGuardrailBlocks client={c} appId="342" />);
    expect(await screen.findByTestId("app-guardrail-blocks")).toBeInTheDocument();
    expect(c.get).toHaveBeenCalledWith("/common/apps/342/guardrail-blocks", { params: { days: 7 } });
    expect(screen.getAllByTestId("app-guardrail-block")).toHaveLength(2);
    expect(screen.getByText("Blocked")).toBeInTheDocument();
    expect(screen.getByText("Tool removed")).toBeInTheDocument();
    expect(screen.getByText("guardrail_too_large_to_scan")).toBeInTheDocument();
    expect(screen.getByText("tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b")).toBeInTheDocument();
  });

  it("renders nothing when there are no blocks or the call fails", async () => {
    const empty = client([]);
    const { container, rerender } = render(<AppGuardrailBlocks client={empty} appId="1" />);
    await waitFor(() => expect(empty.get).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
    const failing = client([], true);
    rerender(<AppGuardrailBlocks client={failing} appId="2" />);
    await waitFor(() => expect(failing.get).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
