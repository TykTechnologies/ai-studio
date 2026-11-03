import React, { useState } from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import GuardrailJudgeSection, { MIN_JUDGE_OPINIONS, countOpinions, judgeErrors } from "./GuardrailJudgeSection";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const apiClient = require("../../utils/apiClient").default;

const llmRows = [
  { id: "70", attributes: { name: "Haiku judge", vendor: "anthropic", default_model: "claude-haiku-5-5", active: true } },
  { id: "71", attributes: { name: "Decisions", vendor: "systemone", default_model: "s1", active: true } },
  { id: "72", attributes: { name: "Off LLM", vendor: "openai", default_model: "gpt-x", active: false } },
];

let opinionEvents = [];
const opinion = (filter, verdict) => ({
  event_type: "guardrail.judge",
  filter_name: filter,
  metadata: { judge: verdict === "none" ? { verdict: "none", error: "timeout" } : { verdict, reason: "unclear" } },
});

// Harness keeps the judge value the section reports, as the filter form does.
const Harness = ({ initial, shadow, error, onValue, filterName }) => {
  const [value, setValue] = useState(initial);
  return (
    <GuardrailJudgeSection
      value={value}
      shadow={shadow}
      error={error}
      filterName={filterName}
      onChange={(v) => {
        setValue(v);
        onValue(v);
      }}
    />
  );
};

const openSelect = (testId) => {
  // MUI renders the select's button next to its hidden input.
  const input = screen.getByTestId(testId);
  fireEvent.mouseDown(input.parentElement.querySelector('[role="combobox"]'));
  return screen.findByRole("listbox");
};

describe("GuardrailJudgeSection (midsommar-clean-9u1j.13)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) =>
      url === "/compliance/events"
        ? Promise.resolve({ data: { events: opinionEvents } })
        : Promise.resolve({ data: { data: llmRows }, headers: {} })
    );
  });

  test("is collapsed and loads nothing until opened", () => {
    render(<Harness onValue={() => {}} />);
    expect(screen.getByText("Second opinion (LLM judge)")).toBeInTheDocument();
    expect(screen.queryByTestId("guardrail-judge-mode")).not.toBeVisible();
    expect(apiClient.get).not.toHaveBeenCalled();
  });

  test("record only, then picking an LLM fills its default model", async () => {
    const onValue = jest.fn();
    render(<Harness onValue={onValue} />);
    fireEvent.click(screen.getByText("Second opinion (LLM judge)"));
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/llms", expect.anything()));

    fireEvent.click(within(await openSelect("guardrail-judge-mode")).getByText("Record only"));
    expect(onValue).toHaveBeenLastCalledWith({ mode: "record" });
    // Without an LLM the section says what is missing.
    expect(screen.getByText("Choose the LLM that gives the second opinion")).toBeInTheDocument();

    const llmSelect = screen.getByLabelText("Judge LLM");
    fireEvent.mouseDown(llmSelect);
    const list = await screen.findByRole("listbox");
    // Chat-capable, active LLMs only.
    expect(within(list).queryByText(/Decisions/)).not.toBeInTheDocument();
    expect(within(list).queryByText(/Off LLM/)).not.toBeInTheDocument();
    fireEvent.click(within(list).getByText(/Haiku judge/));
    expect(onValue).toHaveBeenLastCalledWith({ mode: "record", llm_id: 70, model: "claude-haiku-5-5" });
    expect(screen.getByTestId("guardrail-judge-model")).toHaveValue("claude-haiku-5-5");

    fireEvent.change(screen.getByTestId("guardrail-judge-timeout"), { target: { value: "1500" } });
    expect(onValue).toHaveBeenLastCalledWith({ mode: "record", llm_id: 70, model: "claude-haiku-5-5", timeout_ms: 1500 });

    // Off removes the judge from the config.
    fireEvent.click(within(await openSelect("guardrail-judge-mode")).getByText("Off"));
    expect(onValue).toHaveBeenLastCalledWith(undefined);
  });

  test("soften blocks needs enough recorded opinions, and never on a shadow filter", async () => {
    opinionEvents = [
      ...Array.from({ length: MIN_JUDGE_OPINIONS - 1 }, () => opinion("Wolf", "benign")),
      opinion("Wolf", "none"),
      opinion("Other", "benign"),
    ];
    const { unmount } = render(<Harness initial={{ mode: "record", llm_id: 70 }} filterName="Wolf" onValue={() => {}} />);
    expect(await screen.findByTestId("guardrail-judge-mode-help")).toHaveTextContent(
      `${MIN_JUDGE_OPINIONS - 1} opinions recorded in the last 30 days (benign on ${MIN_JUDGE_OPINIONS - 1})`
    );
    let list = await openSelect("guardrail-judge-mode");
    expect(within(list).getByText("Soften blocks").closest("li")).toHaveAttribute("aria-disabled", "true");
    fireEvent.keyDown(list, { key: "Escape" });
    unmount();

    // Enough opinions: it can be picked, with the warning.
    opinionEvents.push(opinion("Wolf", "attack"));
    const onValue = jest.fn();
    const second = render(<Harness initial={{ mode: "record", llm_id: 70 }} filterName="Wolf" onValue={onValue} />);
    await waitFor(() =>
      expect(screen.getByTestId("guardrail-judge-mode-help")).toHaveTextContent(`${MIN_JUDGE_OPINIONS} opinions`)
    );
    list = await openSelect("guardrail-judge-mode");
    fireEvent.click(within(list).getByText("Soften blocks"));
    expect(onValue).toHaveBeenLastCalledWith({ mode: "soften", llm_id: 70 });
    expect(screen.getByTestId("guardrail-judge-soften-warning")).toHaveTextContent("only as trustworthy as the gateways");
    second.unmount();

    // A shadow filter: never.
    render(<Harness initial={{ mode: "record", llm_id: 70 }} filterName="Wolf" shadow onValue={() => {}} />);
    expect(await screen.findByText(/Haiku judge/)).toBeInTheDocument();
    list = await openSelect("guardrail-judge-mode");
    expect(within(list).getByText("Soften blocks").closest("li")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByText(/shadow filter changes no traffic/)).toBeInTheDocument();
  });

  test("countOpinions counts the filter's opinions with a verdict", () => {
    expect(
      countOpinions([opinion("Wolf", "benign"), opinion("Wolf", "attack"), opinion("Wolf", "none"), opinion("X", "benign"), { event_type: "pii" }], "Wolf")
    ).toEqual({ judged: 2, benign: 1 });
  });


  test("shows a validation error from the API, opened", () => {
    render(
      <Harness
        error={'invalid filter: judge LLM "Public" has privacy score 10, below the 90 of LLM "On-prem"'}
        onValue={() => {}}
      />
    );
    expect(screen.getByTestId("guardrail-judge-error")).toBeVisible();
    expect(screen.getByTestId("guardrail-judge-error")).toHaveTextContent("privacy score 10");
  });

  test("judgeErrors mirrors the server's checks", () => {
    expect(judgeErrors(undefined)).toEqual({});
    expect(judgeErrors({ mode: "off" })).toEqual({});
    expect(judgeErrors({ mode: "record" }).llm_id).toBeTruthy();
    expect(judgeErrors({ mode: "record", llm_id: 1, timeout_ms: 30001 }).timeout_ms).toBeTruthy();
    expect(judgeErrors({ mode: "record", llm_id: 1, timeout_ms: 3000 })).toEqual({});
  });
});
