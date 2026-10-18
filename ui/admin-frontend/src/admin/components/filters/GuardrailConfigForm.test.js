import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import GuardrailConfigForm, {
  budgetErrors,
  effectiveToolAction,
  effectiveToolCallAction,
  emptyGuardrailConfig,
  scansToolCalls,
  scansToolResults,
} from "./GuardrailConfigForm";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const apiClient = require("../../utils/apiClient").default;

const providers = [
  {
    name: "builtin",
    display_name: "Built-in pattern library",
    description: "Runs in-process.",
    redacts: true,
    local: true,
    available: true,
    detectors: [
      { name: "secrets", category: "secrets", label: "Credentials and API keys" },
      { name: "pii", category: "pii", label: "Personal data" },
    ],
    connection_fields: [],
  },
  {
    name: "azure_content_safety",
    display_name: "Azure AI Content Safety",
    description: "Prompt Shields and moderation.",
    redacts: false,
    available: false,
    detectors: [
      { name: "prompt_attack", category: "injection", label: "Prompt Shields" },
      { name: "Hate", category: "moderation", label: "Hate", has_threshold: true, threshold_hint: "severity 0..6" },
    ],
    connection_fields: [
      { name: "endpoint", label: "Endpoint", required: true, example: "https://x" },
      { name: "api_key", label: "Subscription key", required: true, secret: true },
    ],
  },
];

// A stateful harness: the form is controlled, so the test must feed changes
// back in the way FilterForm does.
const Harness = ({ initial, responseFilter = false, onChange = () => {} }) => {
  const [value, setValue] = React.useState(initial);
  return (
    <ThemeProvider theme={testTheme}>
      <GuardrailConfigForm
        value={value}
        responseFilter={responseFilter}
        onChange={(next) => {
          setValue(next);
          onChange(next);
        }}
      />
    </ThemeProvider>
  );
};

describe("GuardrailConfigForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({ data: providers });
  });

  it("lists the built-in provider's detectors and toggles them into the config", async () => {
    const onChange = jest.fn();
    render(<Harness initial={emptyGuardrailConfig()} onChange={onChange} />);

    const secrets = await screen.findByTestId("guardrail-detector-secrets");
    fireEvent.click(secrets);
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ detectors: [{ name: "secrets" }] }));

    fireEvent.click(screen.getByTestId("guardrail-detector-pii"));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ detectors: [{ name: "secrets" }, { name: "pii" }] })
    );

    fireEvent.click(screen.getByTestId("guardrail-detector-secrets"));
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ detectors: [{ name: "pii" }] }));
  });

  it("switching provider resets detectors and shows its connection fields", async () => {
    const onChange = jest.fn();
    render(
      <Harness initial={{ ...emptyGuardrailConfig(), detectors: [{ name: "secrets" }] }} onChange={onChange} />
    );
    await screen.findByTestId("guardrail-detector-secrets");

    fireEvent.change(screen.getByTestId("guardrail-provider"), { target: { value: "azure_content_safety" } });

    await waitFor(() => expect(screen.getByTestId("guardrail-connection-endpoint")).toBeInTheDocument());
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ provider: "azure_content_safety", detectors: [], connection: {} })
    );
    expect(screen.getByText(/not implemented in this edition/i)).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("guardrail-connection-api_key"), { target: { value: "$SECRET/ACS" } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ connection: { api_key: "$SECRET/ACS" } }));
  });

  it("records a threshold for a detector that has one", async () => {
    const onChange = jest.fn();
    render(
      <Harness
        initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety", detectors: [{ name: "Hate" }] }}
        onChange={onChange}
      />
    );
    const threshold = await screen.findByTestId("guardrail-threshold-Hate");
    fireEvent.change(threshold, { target: { value: "2" } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ detectors: [{ name: "Hate", threshold: 2 }] }));
  });

  it("disables redact for a provider that cannot rewrite text", async () => {
    render(<Harness initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety" }} />);
    await screen.findByTestId("guardrail-connection-endpoint");

    // Open the action select and check the redact option is disabled.
    fireEvent.mouseDown(screen.getByLabelText(/on detection/i));
    const redact = await screen.findByRole("option", { name: /redact/i });
    expect(redact).toHaveAttribute("aria-disabled", "true");
  });

  it("explains that redact degrades to log on a response filter", async () => {
    render(<Harness initial={{ ...emptyGuardrailConfig(), on_detect: "redact" }} responseFilter />);
    await screen.findByTestId("guardrail-detector-secrets");
    expect(screen.getByTestId("guardrail-response-redact-warning")).toHaveTextContent(/does not change responses/i);
    // Nothing is rewritten on a response, so there is no style to choose.
    expect(screen.queryByLabelText(/redaction style/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/messages to inspect/i)).not.toBeInTheDocument();
  });

  // Both selects used to render blank until a value was chosen: the default
  // option has value "" and MUI hides that without displayEmpty.
  it("shows the default scope and fail-mode labels for an empty request-filter config", async () => {
    render(<Harness initial={emptyGuardrailConfig()} />);
    await screen.findByTestId("guardrail-detector-secrets");
    expect(screen.getByText("All user messages (default)")).toBeInTheDocument();
    expect(screen.getByText("Block the request (default)")).toBeInTheDocument();
    expect(screen.getByText(/Fail closed blocks every request while the provider is unreachable/)).toBeInTheDocument();
  });

  it("names the response-filter fail-mode default as letting the response through", async () => {
    render(<Harness initial={emptyGuardrailConfig()} responseFilter />);
    await screen.findByTestId("guardrail-detector-secrets");
    expect(screen.getByText("Let the response through (default)")).toBeInTheDocument();
  });

  it("explains that connection URLs resolve from the gateway's network, not the browser", async () => {
    render(<Harness initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety" }} />);
    await screen.findByTestId("guardrail-connection-endpoint");
    const endpointHelp = screen.getByTestId("guardrail-connection-endpoint").closest(".MuiFormControl-root");
    expect(endpointHelp).toHaveTextContent(
      "Resolved from the gateway's network, not your browser: localhost means the gateway container itself."
    );
    const keyHelp = screen.getByTestId("guardrail-connection-api_key").closest(".MuiFormControl-root");
    expect(keyHelp).not.toHaveTextContent("Resolved from the gateway's network");
  });
});

// 4uc.8: tool-result scanning, quarantine, the verdict cache and budgets.
describe("GuardrailConfigForm: tool results", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({ data: providers });
  });

  const lastConfig = (onChange) => onChange.mock.calls[onChange.mock.calls.length - 1][0];

  it("scans tool results on a new filter and quarantines them under Block, without writing tool_results", async () => {
    const onChange = jest.fn();
    render(<Harness initial={emptyGuardrailConfig()} onChange={onChange} />);
    await screen.findByTestId("guardrail-detector-secrets");

    expect(screen.getByTestId("guardrail-tool-results-scan")).toBeChecked();
    expect(screen.getByText("Quarantine (default for Block filters)")).toBeInTheDocument();
    expect(screen.getByText(/The whole tool result is replaced with the placeholder/)).toBeInTheDocument();
    // Quarantine is in play, so its placeholder can be set.
    expect(screen.getByTestId("guardrail-quarantine-placeholder")).toHaveAttribute(
      "placeholder",
      "[content removed by policy: {{detector}} detected in {{source}}]"
    );

    // An unrelated change leaves the server default (absent means scan) alone.
    fireEvent.click(screen.getByTestId("guardrail-detector-secrets"));
    expect(lastConfig(onChange).tool_results).toBeUndefined();
    expect(lastConfig(onChange).quarantine).toBeUndefined();
  });

  it("treats a stored config without tool_results as scanning", async () => {
    render(<Harness initial={{ provider: "builtin", detectors: [{ name: "secrets" }], on_detect: "log" }} />);
    await screen.findByTestId("guardrail-detector-secrets");
    expect(screen.getByTestId("guardrail-tool-results-scan")).toBeChecked();
    expect(screen.getByText("Same as the filter (Log only)")).toBeInTheDocument();
    // Log has no quarantine anywhere: no placeholder to set.
    expect(screen.queryByTestId("guardrail-quarantine-placeholder")).not.toBeInTheDocument();
  });

  it("writes scan false when turned off, and removes tool_results when turned back on", async () => {
    const onChange = jest.fn();
    render(<Harness initial={emptyGuardrailConfig()} onChange={onChange} />);
    const scan = await screen.findByTestId("guardrail-tool-results-scan");

    fireEvent.click(scan);
    expect(lastConfig(onChange).tool_results).toEqual({ scan: false });
    expect(screen.getByTestId("guardrail-tool-action")).toBeDisabled();

    fireEvent.click(scan);
    expect(lastConfig(onChange).tool_results).toBeUndefined();
  });

  it("sets and clears a tool-result action override", async () => {
    const onChange = jest.fn();
    render(<Harness initial={emptyGuardrailConfig()} onChange={onChange} />);
    await screen.findByTestId("guardrail-detector-secrets");

    fireEvent.change(screen.getByTestId("guardrail-tool-action"), { target: { value: "block" } });
    expect(lastConfig(onChange).tool_results).toEqual({ on_detect: "block" });
    expect(screen.getByText(/The client resends the tool result on its next turn/)).toBeInTheDocument();
    // Nothing is quarantined any more.
    expect(screen.queryByTestId("guardrail-quarantine-placeholder")).not.toBeInTheDocument();

    fireEvent.change(screen.getByTestId("guardrail-tool-action"), { target: { value: "" } });
    expect(lastConfig(onChange).tool_results).toBeUndefined();
  });

  it("sets the quarantine placeholder, and removes it when cleared so the default applies", async () => {
    const onChange = jest.fn();
    render(<Harness initial={emptyGuardrailConfig()} onChange={onChange} />);
    const placeholder = await screen.findByTestId("guardrail-quarantine-placeholder");

    fireEvent.change(placeholder, { target: { value: "[removed from {{source}}]" } });
    expect(lastConfig(onChange).quarantine).toEqual({ placeholder: "[removed from {{source}}]" });

    // The server rejects an empty placeholder; clearing the field drops the key.
    fireEvent.change(placeholder, { target: { value: "" } });
    expect(lastConfig(onChange).quarantine).toBeUndefined();
  });

  it("offers Quarantine as the filter's action on a request filter", async () => {
    render(<Harness initial={emptyGuardrailConfig()} />);
    await screen.findByTestId("guardrail-detector-secrets");
    fireEvent.mouseDown(screen.getByLabelText(/on detection/i));
    expect(await screen.findByRole("option", { name: "Quarantine" })).toBeInTheDocument();
  });

  it("offers no quarantine, tool-result, cache or budget settings on a response filter", async () => {
    render(<Harness initial={emptyGuardrailConfig()} responseFilter />);
    await screen.findByTestId("guardrail-detector-secrets");

    fireEvent.mouseDown(screen.getByLabelText(/on detection/i));
    await screen.findByRole("option", { name: "Block" });
    expect(screen.queryByRole("option", { name: "Quarantine" })).not.toBeInTheDocument();

    expect(screen.queryByTestId("guardrail-tool-results-scan")).not.toBeInTheDocument();
    expect(screen.queryByTestId("guardrail-tool-action")).not.toBeInTheDocument();
    expect(screen.queryByTestId("guardrail-quarantine-placeholder")).not.toBeInTheDocument();
    expect(screen.queryByTestId("guardrail-cache")).not.toBeInTheDocument();
  });

  it("disables Redact as the tool-result action for a provider that cannot rewrite", async () => {
    render(<Harness initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety" }} />);
    await screen.findByTestId("guardrail-connection-endpoint");
    fireEvent.mouseDown(screen.getByLabelText(/action on tool results/i));
    const redact = await screen.findByRole("option", { name: /redact/i });
    expect(redact).toHaveAttribute("aria-disabled", "true");
  });

  it("warns that a remote provider receives tool output, and offers budgets for it", async () => {
    render(<Harness initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety" }} />);
    await screen.findByTestId("guardrail-connection-endpoint");
    expect(screen.getByTestId("guardrail-tool-results-remote")).toHaveTextContent(/tool output is sent to it/);
    expect(screen.getByTestId("guardrail-budget-max_scan_chars")).toBeInTheDocument();
    expect(screen.getByTestId("guardrail-budget-time_ms")).toHaveAttribute("placeholder", "8000");
  });

  it("offers the cache but no budgets for the in-process library", async () => {
    render(<Harness initial={emptyGuardrailConfig()} />);
    await screen.findByTestId("guardrail-detector-secrets");
    expect(screen.queryByTestId("guardrail-tool-results-remote")).not.toBeInTheDocument();
    expect(screen.getByTestId("guardrail-cache")).toBeInTheDocument();
    expect(screen.queryByTestId("guardrail-budget-max_scan_chars")).not.toBeInTheDocument();
  });

  it("writes cache false only when turned off", async () => {
    const onChange = jest.fn();
    render(<Harness initial={emptyGuardrailConfig()} onChange={onChange} />);
    const cache = await screen.findByTestId("guardrail-cache");
    expect(cache).toBeChecked();
    fireEvent.click(cache);
    expect(lastConfig(onChange).cache).toBe(false);
    fireEvent.click(cache);
    expect(lastConfig(onChange).cache).toBeUndefined();
  });

  it("writes budget values the admin sets, drops cleared ones and names one out of range", async () => {
    const onChange = jest.fn();
    render(
      <Harness
        initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety", timeout_ms: 5000 }}
        onChange={onChange}
      />
    );
    const timeBudget = await screen.findByTestId("guardrail-budget-time_ms");

    fireEvent.change(timeBudget, { target: { value: "3000" } });
    expect(lastConfig(onChange).budget).toEqual({ time_ms: 3000 });
    expect(screen.getByText("Between the timeout (5000) and 300000")).toBeInTheDocument();

    fireEvent.change(timeBudget, { target: { value: "" } });
    expect(lastConfig(onChange).budget).toBeUndefined();
  });
});

// 4uc.60: the defaults are shown as placeholders, which MUI hides under an
// unshrunk label until the field has focus.
describe("GuardrailConfigForm: defaults are visible", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({ data: providers });
  });

  const labelOf = (testId) =>
    screen.getByTestId(testId).closest(".MuiFormControl-root").querySelector("label");

  it("keeps the labels of the quarantine placeholder and budget fields shrunk", async () => {
    render(<Harness initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety" }} />);
    await screen.findByTestId("guardrail-connection-endpoint");
    ["guardrail-quarantine-placeholder", "guardrail-budget-max_scan_chars", "guardrail-budget-time_ms"].forEach((id) =>
      expect(labelOf(id)).toHaveAttribute("data-shrink", "true")
    );
  });
});

describe("GuardrailConfigForm tool calls", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({ data: providers });
  });

  it("saves only what the admin set, and removes the key when scanning is turned off", async () => {
    const onChange = jest.fn();
    render(<Harness initial={{ ...emptyGuardrailConfig(), provider: "builtin", on_detect: "block" }} onChange={onChange} />);
    const scan = await screen.findByTestId("guardrail-tool-calls-scan");
    expect(scan).not.toBeChecked();
    expect(screen.getByTestId("guardrail-tool-calls-include-system")).toBeDisabled();

    fireEvent.click(scan);
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ tool_calls: { scan: true } }));
    fireEvent.click(screen.getByTestId("guardrail-tool-calls-include-system"));
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ tool_calls: { scan: true, include_system: true } }));
    fireEvent.click(screen.getByTestId("guardrail-tool-calls-include-system"));
    fireEvent.click(screen.getByTestId("guardrail-tool-calls-scan"));
    expect(onChange.mock.calls[onChange.mock.calls.length - 1][0].tool_calls).toBeUndefined();
  });

  it("is offered on response filters too, and warns that a remote provider receives the arguments", async () => {
    render(
      <Harness
        responseFilter
        initial={{ ...emptyGuardrailConfig(), provider: "azure_content_safety", tool_calls: { scan: true } }}
      />
    );
    expect(await screen.findByTestId("guardrail-tool-calls-scan")).toBeChecked();
    expect(screen.getByTestId("guardrail-tool-calls-remote")).toBeInTheDocument();
    expect(screen.getByText(/never sent to the client/)).toBeInTheDocument();
  });
});

describe("guardrail config helpers", () => {
  it("maps block to quarantine for tool results unless overridden", () => {
    expect(effectiveToolAction({ on_detect: "block" })).toBe("quarantine");
    expect(effectiveToolAction({ on_detect: "redact" })).toBe("redact");
    expect(effectiveToolAction({ on_detect: "block", tool_results: { on_detect: "block" } })).toBe("block");
  });

  it("scans unless scan is explicitly false", () => {
    expect(scansToolResults({})).toBe(true);
    expect(scansToolResults({ tool_results: {} })).toBe(true);
    expect(scansToolResults({ tool_results: { scan: false } })).toBe(false);
  });

  it("scans tool calls only when scan is true, and defaults their action to block or log", () => {
    expect(scansToolCalls({})).toBe(false);
    expect(scansToolCalls({ tool_calls: { scan: true } })).toBe(true);
    expect(effectiveToolCallAction({ on_detect: "block" })).toBe("block");
    expect(effectiveToolCallAction({ on_detect: "quarantine" })).toBe("log");
    expect(effectiveToolCallAction({ on_detect: "redact" })).toBe("log");
    expect(effectiveToolCallAction({ on_detect: "log", tool_calls: { on_detect: "block" } })).toBe("block");
  });

  it("checks single_call_chars against max_scan_chars", () => {
    expect(budgetErrors({ budget: { max_scan_chars: 1000, single_call_chars: 2000 } })).toHaveProperty(
      "single_call_chars"
    );
    expect(budgetErrors({ budget: { single_call_chars: 32768 } })).toEqual({});
  });
});
