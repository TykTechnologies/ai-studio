import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import GuardrailConfigForm, { emptyGuardrailConfig } from "./GuardrailConfigForm";

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
    expect(screen.getByText(/responses are block-only/i)).toBeInTheDocument();
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
