import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import ScriptTestPanel from "./ScriptTestPanel";

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
});
