import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import FailoverCell, { formatFailover } from "../FailoverCell";

describe("formatFailover", () => {
  it("shows a dash for a primary attempt", () => {
    expect(formatFailover({ failover_attempt: 0 })).toBe("-");
    expect(formatFailover({})).toBe("-");
    expect(formatFailover(undefined)).toBe("-");
  });

  it("names the rung and the primary it failed over from", () => {
    expect(formatFailover({ failover_attempt: 1, failover_from_llm_id: 3 })).toBe(
      "attempt 1 (from LLM #3)",
    );
  });

  it("still marks a rung whose from-id is absent", () => {
    expect(formatFailover({ failover_attempt: 2 })).toBe("attempt 2");
  });
});

describe("FailoverCell", () => {
  it("renders the fallback marker", () => {
    render(<FailoverCell attributes={{ failover_attempt: 1, failover_from_llm_id: 3 }} />);
    expect(screen.getByTestId("proxy-log-failover")).toHaveTextContent("attempt 1 (from LLM #3)");
  });

  it("renders a dash for the primary attempt", () => {
    render(<FailoverCell attributes={{ failover_attempt: 0, response_code: 503 }} />);
    expect(screen.getByTestId("proxy-log-failover")).toHaveTextContent("-");
  });
});
