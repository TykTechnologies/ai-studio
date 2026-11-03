import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import SoftenDialog from "./SoftenDialog";
import { softenTarget } from "./softenTarget";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), put: jest.fn() },
}));
let mockFullAdmin = true;
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: () => ({ isFullAdmin: mockFullAdmin, can: () => true, canAny: () => true, canAll: () => true }),
}));

const apiClient = require("../../utils/apiClient").default;

const meta = {
  max_approved: 20,
  actions: { text_user: ["log", "quarantine"], text_system: ["log", "quarantine"], tool_result: ["log", "quarantine"], tool_call: ["log"], tool_definition: ["log", "remove"] },
  floors: {
    read_only: { text_user: "log", text_system: "log", tool_result: "log", tool_call: "log", tool_definition: "log" },
    read_tools: { text_user: "log", text_system: "log", tool_result: "quarantine", tool_call: "log", tool_definition: "log" },
    acting: { text_user: "quarantine", text_system: "quarantine", tool_result: "", tool_call: "", tool_definition: "" },
  },
};

const event = (extra = {}) => ({
  id: 501,
  app_id: 9,
  app_name: "Visor",
  filter_name: "Wolf Defender",
  filter_scope: "proxy_request",
  event_type: "guardrail.prompt_attack",
  metadata: { filter_id: 13, detector: "prompt_attack", action: "block", segments: [{ index: 0, role: "user", kind: "text", sha256: "ab" }] },
  ...extra,
});

const serve = (capability) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/apps/9/guardrail-capability") return Promise.resolve({ data: { data: capability } });
    if (url === "/apps/9/guardrail-exceptions") return Promise.resolve({ data: { data: [], meta } });
    return Promise.reject(new Error("unexpected " + url));
  });
};

const renderDialog = (ev = event()) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <SoftenDialog open target={softenTarget(ev)} onClose={() => {}} />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("SoftenDialog (midsommar-clean-9u1j.7)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockFullAdmin = true;
  });

  it("is pre-filled from the event and hides the capability step when attested", async () => {
    serve({ value: "read_only", effective: "read_only", suggestion: "read_only", reasons: [] });
    renderDialog();
    expect(screen.getByTestId("soften-scope")).toHaveTextContent("Wolf Defender flagged prompt_attack in the app's own messages of Visor");
    expect(await screen.findByTestId("soften-justification")).toBeInTheDocument();
    expect(screen.queryByTestId("soften-capability")).not.toBeInTheDocument();
    // The softest allowed is the default.
    expect(screen.getByRole("combobox", { name: "Instead of blocking" })).toHaveTextContent("Log only");
  });

  it("requires a justification and posts the exception with its source event", async () => {
    serve({ value: "read_only", effective: "read_only", suggestion: "read_only", reasons: [] });
    apiClient.post.mockResolvedValue({ data: { data: { id: 77, status: "approved", action: "log" } } });
    renderDialog();
    await screen.findByTestId("soften-justification");
    expect(screen.getByTestId("soften-submit")).toBeDisabled();
    fireEvent.change(screen.getByTestId("soften-justification"), { target: { value: "PR content quotes test vectors" } });
    fireEvent.click(screen.getByTestId("soften-submit"));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/apps/9/guardrail-exceptions", {
        filter_id: 13, detector: "prompt_attack", channel: "text_user", tool_name: "", action: "log",
        justification: "PR content quotes test vectors", expires_after_days: 30, source_event: 501,
      })
    );
    expect(apiClient.put).not.toHaveBeenCalled();
    expect(await screen.findByTestId("soften-result")).toHaveTextContent("Exception #77 applies now");
  });

  it("asks what an unattested app can do, with the suggestion, and attests it first", async () => {
    serve({ value: "", effective: "acting", suggestion: "read_only", reasons: ["No tools, datasources or MCP servers attached"] });
    apiClient.put.mockResolvedValue({ data: { data: { value: "read_only" } } });
    apiClient.post.mockResolvedValue({ data: { data: { id: 78, status: "approved", action: "log" } } });
    renderDialog();
    const step = await screen.findByTestId("soften-capability");
    expect(step).toHaveTextContent("No tools, datasources or MCP servers attached");
    expect(within(step).getByLabelText(/Read-only \(suggested\)/)).toBeChecked();
    expect(screen.getByRole("combobox", { name: "Instead of blocking" })).toHaveTextContent("Log only");
    // Attesting acting instead limits the choice to removing the part.
    fireEvent.click(within(step).getByLabelText(/^Acting/));
    expect(screen.getByRole("combobox", { name: "Instead of blocking" })).toHaveTextContent("Remove the flagged part");
    fireEvent.click(within(step).getByLabelText(/Read-only/));
    fireEvent.change(screen.getByTestId("soften-justification"), { target: { value: "ok" } });
    fireEvent.click(screen.getByTestId("soften-submit"));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalledWith("/apps/9/guardrail-capability", { value: "read_only" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
  });

  it("offers only what the floor allows, and nothing on an acting app's tool results", async () => {
    serve({ value: "read_tools", effective: "read_tools", suggestion: "read_tools", reasons: [] });
    const toolEvent = event({ metadata: { filter_id: 13, detector: "prompt_attack", segments: [{ index: 2, role: "tool", kind: "tool_result", tool: "read_file" }] } });
    const { unmount } = renderDialog(toolEvent);
    await screen.findByTestId("soften-justification");
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Instead of blocking" }));
    const list = within(await screen.findByRole("listbox"));
    expect(list.queryByText("Log only")).not.toBeInTheDocument();
    expect(list.getByText("Remove the flagged part")).toBeInTheDocument();
    unmount();

    serve({ value: "acting", effective: "acting", suggestion: "acting", reasons: [] });
    renderDialog(toolEvent);
    expect(await screen.findByTestId("soften-action-help")).toHaveTextContent("cannot be softened here");
    fireEvent.change(screen.getByTestId("soften-justification"), { target: { value: "x" } });
    expect(screen.getByTestId("soften-submit")).toBeDisabled();
  });

  it("proposes when the user is not a full administrator, without attesting", async () => {
    mockFullAdmin = false;
    serve({ value: "", effective: "acting", suggestion: "read_only", reasons: [] });
    apiClient.post.mockResolvedValue({ data: { data: { id: 79, status: "proposed", action: "quarantine" } } });
    renderDialog();
    expect(await screen.findByTestId("soften-capability")).toHaveTextContent("treated as acting");
    // Unattested is acting: own text can only have the part removed.
    expect(screen.getByRole("combobox", { name: "Instead of blocking" })).toHaveTextContent("Remove the flagged part");
    fireEvent.change(screen.getByTestId("soften-justification"), { target: { value: "ok" } });
    fireEvent.click(screen.getByTestId("soften-submit"));
    expect(await screen.findByTestId("soften-result")).toHaveTextContent("Exception #79 is proposed");
    expect(apiClient.put).not.toHaveBeenCalled();
  });

  it("shows the server's refusal", async () => {
    serve({ value: "read_only", effective: "read_only", suggestion: "read_only", reasons: [] });
    apiClient.post.mockRejectedValue({ response: { data: { errors: [{ detail: "compliance event 501 was not recorded by filter 13" }] } } });
    renderDialog();
    fireEvent.change(await screen.findByTestId("soften-justification"), { target: { value: "x" } });
    fireEvent.click(screen.getByTestId("soften-submit"));
    expect(await screen.findByTestId("soften-error")).toHaveTextContent("not recorded by filter 13");
  });
});

describe("softenTarget", () => {
  it("only offers a detector's verdict on an app's request from a named filter", () => {
    expect(softenTarget(event())).toMatchObject({ filterId: 13, detector: "prompt_attack", channel: "text_user", appId: 9, eventId: 501 });
    expect(softenTarget(event({ app_id: 0 }))).toBeNull();
    expect(softenTarget(event({ metadata: { detector: "prompt_attack", roles: ["user"] } }))).toBeNull();
    expect(softenTarget(event({ event_type: "guardrail.unscanned" }))).toBeNull();
    expect(softenTarget(event({ event_type: "guardrail.judge" }))).toBeNull();
    expect(softenTarget(event({ event_type: "pii_redacted" }))).toBeNull();
    expect(softenTarget(event({ filter_scope: "proxy_response" }))).toBeNull();
    expect(softenTarget(event({ metadata: { filter_id: 13, roles: ["user"] } }))).toBeNull();
    expect(softenTarget(event({ metadata: { filter_id: 13, detector: "x", roles: ["system"] } })).channel).toBe("text_system");
    expect(
      softenTarget(event({ metadata: { filter_id: 13, detector: "x", kinds: ["tool_call"], tool_names: ["shell"] } }))
    ).toMatchObject({ channel: "tool_call", toolName: "shell" });
  });
});
