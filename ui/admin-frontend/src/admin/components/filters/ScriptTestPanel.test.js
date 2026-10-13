import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import ScriptTestPanel, { TOOL_RESULT_SAMPLES } from "./ScriptTestPanel";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { post: jest.fn() },
}));

const apiClient = require("../../utils/apiClient").default;

const renderPanel = (props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <ScriptTestPanel script="output := { block: false }" {...props} />
    </ThemeProvider>
  );

// The accordion keeps its children mounted, so the controls are in the DOM
// without expanding. There are exactly two buttons -- the accordion header and
// the run control -- and at the time of writing they share the name "Test
// Script" (see #502, E4), so select the run control by position rather than by
// a name that is about to change.
const runButton = (container) => {
  const buttons = [...container.querySelectorAll("button")];
  expect(buttons.length).toBeGreaterThanOrEqual(2);
  return buttons[buttons.length - 1];
};

const runTest = async (container) => {
  fireEvent.click(runButton(container));
  await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
  return apiClient.post.mock.calls[0][1];
};

describe("ScriptTestPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.post.mockResolvedValue({ data: { success: true, output: {} } });
  });

  // tyk.redact_pattern switches behaviour on whether raw_input parses as JSON.
  // With messages hardcoded to [], pasting a real request body came back with
  // the messages stripped out -- which reads as a catastrophic redaction
  // failure and is nothing of the sort.
  it("derives messages from a raw_input that is a real request body", async () => {
    const { container } = renderPanel();

    const body = JSON.stringify({
      model: "gpt-5",
      messages: [{ role: "user", content: "hello" }],
    });
    fireEvent.change(screen.getByLabelText(/raw input/i), {
      target: { name: "raw_input", value: body },
    });

    const sent = await runTest(container);
    expect(sent.input.messages).toEqual([{ role: "user", content: "hello" }]);
  });

  it("sends no messages when raw_input is plain text", async () => {
    const { container } = renderPanel();
    fireEvent.change(screen.getByLabelText(/raw input/i), {
      target: { name: "raw_input", value: "just some prose" },
    });

    const sent = await runTest(container);
    expect(sent.input.messages).toEqual([]);
  });

  // The panel used to suggest {"app_id": 1, "user_id": 5}. The proxy supplies
  // llm_id, app_id and request_id, and no user_id at all, so a filter written
  // against the suggestion read undefined in production.
  it("pre-fills the context shape the proxy actually sends", async () => {
    const { container } = renderPanel();
    const sent = await runTest(container);

    expect(sent.input.context).toHaveProperty("llm_id");
    expect(sent.input.context).toHaveProperty("app_id");
    expect(sent.input.context).toHaveProperty("request_id");
    expect(sent.input.context).not.toHaveProperty("user_id");
  });

  it("defaults is_chat to false, matching both proxy filter paths", async () => {
    const { container } = renderPanel();
    const sent = await runTest(container);
    expect(sent.input.is_chat).toBe(false);
  });

  it("surfaces compliance events raised by the script", async () => {
    apiClient.post.mockResolvedValue({
      data: {
        success: true,
        output: {
          block: false,
          compliance_events: [
            {
              event_type: "pii_redacted",
              severity: "warning",
              description: "Redacted 2 email addresses",
            },
          ],
        },
      },
    });

    const { container } = renderPanel();
    await runTest(container);

    await waitFor(() => {
      expect(screen.getByText("Compliance events raised:")).toBeInTheDocument();
      expect(
        screen.getByText(/pii_redacted — Redacted 2 email addresses/)
      ).toBeInTheDocument();
    });
  });

  // The server only resolves $SECRET/ and $ENV/ references for a saved filter
  // whose connection block is unchanged, so the panel names the filter being
  // edited and warns when there is nothing saved to name.
  describe("guardrail secret references", () => {
    const guardrail = {
      provider: "lakera",
      detectors: [{ name: "prompt_attack" }],
      on_detect: "block",
      connection: { endpoint: "https://api.lakera.ai/v2/guard", api_key: "$SECRET/LAKERA" },
    };

    it("sends the saved filter id with a guardrail test", async () => {
      const { container } = renderPanel({ kind: "guardrail", config: guardrail, filterId: "42" });

      const sent = await runTest(container);
      expect(sent.kind).toBe("guardrail");
      expect(sent.filter_id).toBe(42);
      expect(screen.queryByText(/Save the filter first/)).not.toBeInTheDocument();
    });

    it("warns that an unsaved filter cannot resolve references, and sends no id", async () => {
      const { container } = renderPanel({ kind: "guardrail", config: guardrail });

      expect(screen.getByText(/Save the filter first/)).toBeInTheDocument();
      const sent = await runTest(container);
      expect(sent.filter_id).toBeUndefined();
    });

    it("does not warn when the connection holds literal values", () => {
      renderPanel({
        kind: "guardrail",
        config: { ...guardrail, connection: { endpoint: "https://api.lakera.ai/v2/guard", api_key: "literal" } },
      });
      expect(screen.queryByText(/Save the filter first/)).not.toBeInTheDocument();
    });
  });
});

// 4uc.8: a guardrail reads a vendor request body block by block when the
// panel names its format, and the result names the tool a finding came from.
describe("ScriptTestPanel: tool results in a guardrail test", () => {
  const guardrail = { provider: "builtin", detectors: [{ name: "injection" }], on_detect: "block" };

  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.post.mockResolvedValue({ data: { success: true, output: {} } });
  });

  it("sends no format by default, as before", async () => {
    const { container } = renderPanel({ kind: "guardrail", config: guardrail, filterType: "request" });
    const sent = await runTest(container);
    expect(sent.input.format).toBeUndefined();
  });

  it("inserts an Anthropic tool-result sample and sends it with its format", async () => {
    const { container } = renderPanel({ kind: "guardrail", config: guardrail, filterType: "request" });
    fireEvent.change(screen.getByTestId("test-panel-format"), { target: { name: "format", value: "anthropic" } });
    fireEvent.click(screen.getByTestId("test-panel-tool-sample"));

    const sent = await runTest(container);
    expect(sent.input.format).toBe("anthropic");
    expect(sent.input.vendor_name).toBe("anthropic");
    const body = JSON.parse(sent.input.raw_input);
    expect(body.messages[2].content[0]).toMatchObject({ type: "tool_result", tool_use_id: "toolu_1" });
  });

  it("defaults the sample to the OpenAI format", async () => {
    const { container } = renderPanel({ kind: "guardrail", config: guardrail, filterType: "request" });
    fireEvent.click(screen.getByTestId("test-panel-tool-sample"));
    const sent = await runTest(container);
    expect(sent.input.format).toBe("openai");
    expect(JSON.parse(sent.input.raw_input).messages[2]).toMatchObject({ role: "tool", tool_call_id: "call_1" });
  });

  it("has a tool-result sample for every format it offers", () => {
    ["openai", "anthropic", "google_ai", "vertex", "ollama", "bedrock"].forEach((f) =>
      expect(TOOL_RESULT_SAMPLES[f]).toBeDefined()
    );
  });

  it("names the tool a finding came from and shows the quarantined body", async () => {
    const rewritten = JSON.stringify({
      messages: [{ role: "tool", tool_call_id: "call_1", content: "[content removed by policy]" }],
    });
    apiClient.post.mockResolvedValue({
      data: {
        success: true,
        output: {
          block: false,
          payload: rewritten,
          compliance_events: [
            {
              event_type: "guardrail.injection.ignore_instructions",
              severity: "warning",
              description: "guardrail \"test-filter\" (builtin): injection detected (1) in tool; action quarantine",
              metadata: { action: "quarantine", roles: ["tool"], kinds: ["tool_result"], tool_names: ["fetch_url"] },
            },
          ],
        },
      },
    });
    const { container } = renderPanel({ kind: "guardrail", config: guardrail, filterType: "request" });
    fireEvent.click(screen.getByTestId("test-panel-tool-sample"));
    await runTest(container);

    expect(await screen.findByText(/\(source: Tool result from fetch_url\)/)).toBeInTheDocument();
    expect(screen.getByTestId("test-panel-rewritten-body")).toHaveTextContent("[content removed by policy]");
  });

  it("offers no body format on a response filter", () => {
    renderPanel({ kind: "guardrail", config: guardrail, filterType: "response" });
    expect(screen.queryByTestId("test-panel-format")).not.toBeInTheDocument();
    expect(screen.queryByTestId("test-panel-tool-sample")).not.toBeInTheDocument();
  });

  // 4uc.11: input.segments is built from raw_input and format, so a script
  // (the scan-tool-results template) needs the format too.
  it("sends the format for a script request filter", async () => {
    const { container } = renderPanel({ filterType: "request" });
    fireEvent.change(screen.getByTestId("test-panel-format"), { target: { name: "format", value: "google_ai" } });
    fireEvent.click(screen.getByTestId("test-panel-tool-sample"));
    const sent = await runTest(container);
    expect(sent.kind).toBe("script");
    expect(sent.input.format).toBe("google_ai");
    expect(JSON.parse(sent.input.raw_input).contents[2].parts[0].functionResponse.name).toBe("fetch_url");
  });

  it("offers no body format to a script response filter", () => {
    renderPanel({ filterType: "response" });
    expect(screen.queryByTestId("test-panel-format")).not.toBeInTheDocument();
  });
});
