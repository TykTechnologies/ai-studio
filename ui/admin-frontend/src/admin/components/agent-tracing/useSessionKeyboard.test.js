import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { SessionDetailProvider, useSessionDetail } from "./SessionDetailContext";
import useSessionKeyboard from "./useSessionKeyboard";
import { detail, ids, spans } from "./testFixture";

// Every key binding of the session detail, on the shared state.

const Probe = () => {
  useSessionKeyboard();
  const s = useSessionDetail();
  return (
    <div>
      <span data-testid="selected">{s.selectedId || ""}</span>
      <span data-testid="drawer">{s.drawerSpanId || ""}</span>
      <span data-testid="view">{s.view}</span>
      <span data-testid="expand">{String(s.expandAll)}</span>
      <span data-testid="find">{String(s.findOpen)}</span>
      <input aria-label="typing" />
      <button type="button">a button</button>
      <div data-span-view="true" tabIndex={0} data-testid="grid" />
    </div>
  );
};

const setup = () =>
  render(
    <SessionDetailProvider detail={detail()} spans={spans} resources={[]}>
      <Probe />
    </SessionDetailProvider>,
  );

const press = (key, opts = {}, target = document.body) => fireEvent.keyDown(target, { key, ...opts });
const selected = () => screen.getByTestId("selected").textContent;

describe("session keyboard model", () => {
  it("j / k move between LLM steps", () => {
    setup();
    press("j");
    expect(selected()).toBe(ids.llm1);
    press("j");
    expect(selected()).toBe(ids.llm2);
    press("j");
    expect(selected()).toBe(ids.llm2);
    press("k");
    expect(selected()).toBe(ids.llm1);
  });

  it("n / N move between tool calls, including inferred client tools", () => {
    setup();
    press("n");
    expect(selected()).toBe(ids.tool1);
    press("n");
    expect(selected()).toBe(ids.client);
    press("N", { shiftKey: true });
    expect(selected()).toBe(ids.tool1);
  });

  it("e / E move between errors and gateway denials; g between gateway events", () => {
    setup();
    press("e");
    expect(selected()).toBe(ids.deny);
    press("e");
    expect(selected()).toBe(ids.llm2);
    press("E", { shiftKey: true });
    expect(selected()).toBe(ids.deny);
    press("j");
    press("j");
    press("g");
    expect(selected()).toBe(ids.llm2, "no gateway event after the last step");
  });

  it("1 and 2 switch views, w toggles expand-all, / and Ctrl-F open find, Esc closes it", () => {
    setup();
    press("2");
    expect(screen.getByTestId("view")).toHaveTextContent("timeline");
    press("1");
    expect(screen.getByTestId("view")).toHaveTextContent("transcript");
    press("3");
    expect(screen.getByTestId("view")).toHaveTextContent("transcript");
    press("w");
    expect(screen.getByTestId("expand")).toHaveTextContent("true");
    press("w");
    expect(screen.getByTestId("expand")).toHaveTextContent("false");
    press("/");
    expect(screen.getByTestId("find")).toHaveTextContent("true");
    press("Escape");
    expect(screen.getByTestId("find")).toHaveTextContent("false");
    press("f", { ctrlKey: true });
    expect(screen.getByTestId("find")).toHaveTextContent("true");
  });

  it("Enter opens the selected span from the span view, moving follows it, Esc closes", () => {
    setup();
    press("j");
    press("Enter", {}, screen.getByRole("button", { name: "a button" }));
    expect(screen.getByTestId("drawer")).toHaveTextContent("", "Enter on a button is the button's");
    press("Enter", {}, screen.getByTestId("grid"));
    expect(screen.getByTestId("drawer")).toHaveTextContent(ids.llm1);
    press("j");
    expect(screen.getByTestId("drawer")).toHaveTextContent(ids.llm2);
    press("Escape");
    expect(screen.getByTestId("drawer")).toHaveTextContent("");
  });

  it("leaves keys typed into a field alone", () => {
    setup();
    press("j", {}, screen.getByLabelText("typing"));
    press("/", {}, screen.getByLabelText("typing"));
    expect(selected()).toBe("");
    expect(screen.getByTestId("find")).toHaveTextContent("false");
  });
});
