import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../../utils/testTheme";
import AppGuardrailExceptions from "./AppGuardrailExceptions";
import { allowedActions, exceptionExpiresAt } from "../../../services/guardrailExceptionService";

jest.mock("../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
let mockAccess = { isFullAdmin: false, granted: () => true };
jest.mock("../../../context/PermissionsContext", () => ({
  usePermissions: () => ({
    isFullAdmin: mockAccess.isFullAdmin,
    can: (p) => mockAccess.granted(p),
    canAny: () => true,
    canAll: () => true,
  }),
}));

const apiClient = require("../../../utils/apiClient").default;

const meta = {
  max_approved: 20,
  actions: {
    text_user: ["log", "quarantine"],
    text_system: ["log", "quarantine"],
    tool_result: ["log", "quarantine"],
    tool_call: ["log"],
    tool_definition: ["log", "remove"],
  },
  floors: {
    read_only: { text_user: "log", text_system: "log", tool_result: "log", tool_call: "log", tool_definition: "log" },
    read_tools: { text_user: "log", text_system: "log", tool_result: "quarantine", tool_call: "log", tool_definition: "log" },
    acting: { text_user: "quarantine", text_system: "quarantine", tool_result: "", tool_call: "", tool_definition: "" },
  },
};

const exception = (id, status, extra = {}) => ({
  id,
  app_id: 7,
  filter_id: 13,
  detector: "prompt_attack",
  channel: "text_user",
  tool_name: "",
  action: "log",
  status,
  justification: "PR content quotes injection test vectors",
  proposed_by: 2,
  approved_by: status === "approved" ? 1 : null,
  approved_at: status === "approved" ? "2026-11-01T10:00:00Z" : null,
  expires_after_days: 30,
  last_hit_at: status === "approved" ? "2026-11-02T10:00:00Z" : null,
  hit_count: status === "approved" ? 17 : 0,
  content_hash: `hash-${id}`,
  ...extra,
});

const approved = exception(1, "approved");
const proposed = exception(2, "proposed", { detector: "", channel: "tool_result", tool_name: "read_file", action: "quarantine" });

const renderTable = (capability = "read_only") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <AppGuardrailExceptions appId="7" capability={capability} />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("AppGuardrailExceptions (midsommar-clean-9u1j.6)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAccess = { isFullAdmin: false, granted: () => true };
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps/7/guardrail-exceptions") return Promise.resolve({ data: { data: [approved, proposed], meta } });
      if (url === "/filters") return Promise.resolve({ data: [{ id: "13", attributes: { name: "Wolf Defender", kind: "guardrail" } }, { id: "14", attributes: { name: "Script", kind: "script" } }] });
      if (url === "/users/1") return Promise.resolve({ data: { data: { attributes: { name: "Ada Admin" } } } });
      return Promise.reject(new Error("not found"));
    });
    apiClient.post.mockResolvedValue({ data: { data: {} } });
    apiClient.delete.mockResolvedValue({});
  });

  it("lists the exceptions with filter, scope, action, status and hits", async () => {
    renderTable();
    const row = await screen.findByTestId("guardrail-exception-1");
    expect(await within(row).findByText("Wolf Defender")).toHaveAttribute("href", "/admin/filters/13");
    expect(row).toHaveTextContent("prompt_attack");
    expect(row).toHaveTextContent("The app's own messages");
    expect(row).toHaveTextContent("Log only");
    expect(row).toHaveTextContent("approved");
    expect(row).toHaveTextContent("17");
    expect(await within(row).findByText("Ada Admin")).toBeInTheDocument();

    const other = screen.getByTestId("guardrail-exception-2");
    expect(other).toHaveTextContent("Any");
    expect(other).toHaveTextContent("Tool results (read_file)");
    expect(other).toHaveTextContent("Remove the flagged part");
    expect(other).toHaveTextContent("Never");
    expect(screen.getByText(/1 of at most 20 approved/)).toBeInTheDocument();
  });

  it("offers approval of a proposed exception to a full administrator only", async () => {
    renderTable();
    const row = await screen.findByTestId("guardrail-exception-2");
    expect(within(row).queryByText("Review and approve")).not.toBeInTheDocument();
    fireEvent.click(within(row).getByText("Review"));
    expect(await screen.findByTestId("gx-review")).toBeInTheDocument();
    expect(screen.queryByTestId("gx-approve")).not.toBeInTheDocument();
  });

  it("approves the reviewed version with its content hash", async () => {
    mockAccess = { isFullAdmin: true, granted: () => true };
    renderTable();
    const row = await screen.findByTestId("guardrail-exception-2");
    fireEvent.click(within(row).getByText("Review and approve"));
    const approve = await screen.findByTestId("gx-approve");
    expect(approve).toBeDisabled();
    fireEvent.click(screen.getByTestId("gx-confirm"));
    fireEvent.click(approve);
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/apps/7/guardrail-exceptions/2/approve", { content_hash: "hash-2" })
    );
    // An approved row has nothing left to approve.
    expect(within(screen.getByTestId("guardrail-exception-1")).queryByText("Review and approve")).not.toBeInTheDocument();
  });

  it("retires and deletes through the API", async () => {
    renderTable();
    const row = await screen.findByTestId("guardrail-exception-1");
    fireEvent.click(within(row).getByText("Retire"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/apps/7/guardrail-exceptions/1/retire"));

    fireEvent.click(within(screen.getByTestId("guardrail-exception-2")).getByText("Delete"));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith("/apps/7/guardrail-exceptions/2"));
  });

  it("hides write actions from a reader", async () => {
    mockAccess = { isFullAdmin: false, granted: () => false };
    renderTable();
    const row = await screen.findByTestId("guardrail-exception-1");
    expect(within(row).queryByText("Retire")).not.toBeInTheDocument();
    expect(within(row).queryByText("Delete")).not.toBeInTheDocument();
    expect(within(row).queryByText("Edit")).not.toBeInTheDocument();
    expect(screen.queryByText("Propose exception")).not.toBeInTheDocument();
  });

  it("proposes an exception with only the actions the capability allows", async () => {
    apiClient.post.mockResolvedValue({ data: { data: { id: 3, status: "proposed" } } });
    renderTable("read_tools");
    await screen.findByTestId("guardrail-exception-1");
    await waitFor(() => expect(screen.getByText("Propose exception")).toBeEnabled());
    fireEvent.click(screen.getByText("Propose exception"));
    // Tool results of an app whose tools read: down to removing the part.
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Where" }));
    fireEvent.click(within(await screen.findByRole("listbox")).getByText("Tool results"));
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Instead of blocking" }));
    const actions = within(await screen.findByRole("listbox"));
    expect(actions.queryByText("Log only")).not.toBeInTheDocument();
    fireEvent.click(actions.getByText("Remove the flagged part"));
    fireEvent.change(screen.getByTestId("gx-tool"), { target: { value: "read_file" } });
    fireEvent.change(screen.getByTestId("gx-justification"), { target: { value: "Fixtures in the repo" } });
    fireEvent.click(screen.getByTestId("gx-save"));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/apps/7/guardrail-exceptions", {
        filter_id: 13, detector: "", channel: "tool_result", tool_name: "read_file", action: "quarantine",
        justification: "Fixtures in the repo", expires_after_days: 30,
      })
    );
    expect(await screen.findByText(/Exception #3 is proposed/)).toBeInTheDocument();
  });

  it("mirrors the floor table and the lapse rule", () => {
    expect(allowedActions(meta, "read_only", "tool_result")).toEqual(["log", "quarantine"]);
    expect(allowedActions(meta, "read_tools", "tool_result")).toEqual(["quarantine"]);
    expect(allowedActions(meta, "acting", "tool_result")).toEqual([]);
    expect(allowedActions(meta, "acting", "text_user")).toEqual(["quarantine"]);
    expect(allowedActions(meta, undefined, "text_user")).toEqual(["quarantine"]);
    expect(exceptionExpiresAt(proposed)).toBeNull();
    expect(exceptionExpiresAt(approved).toISOString()).toBe("2026-12-02T10:00:00.000Z");
    // Approved again after its last hit: counted from the approval.
    expect(
      exceptionExpiresAt({ ...approved, approved_at: "2026-11-05T10:00:00Z" }).toISOString()
    ).toBe("2026-12-05T10:00:00.000Z");
  });

  // midsommar-clean-9u1j.54: an alert exception's event name is part of what
  // is approved (its content hash), so the table and the review show it, and
  // the editor asks for it.
  it("shows and asks for the event an alert exception raises", async () => {
    mockAccess = { isFullAdmin: true, granted: () => true };
    const alertMeta = { ...meta, actions: { ...meta.actions, text_user: ["log", "alert", "quarantine"] } };
    const alerting = exception(5, "proposed", { action: "alert", alert_event: "visor-pr-flag" });
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps/7/guardrail-exceptions") return Promise.resolve({ data: { data: [alerting], meta: alertMeta } });
      if (url === "/filters") return Promise.resolve({ data: [{ id: "13", attributes: { name: "Wolf Defender", kind: "guardrail" } }] });
      return Promise.reject(new Error("not found"));
    });
    renderTable();
    const row = await screen.findByTestId("guardrail-exception-5");
    expect(row).toHaveTextContent("Log and raise event (visor-pr-flag)");
    fireEvent.click(within(row).getByText("Review and approve"));
    expect(await screen.findByTestId("gx-review")).toHaveTextContent("guardrail.alert.visor-pr-flag");
    fireEvent.click(screen.getByText("Close"));

    apiClient.post.mockResolvedValue({ data: { data: { id: 6, status: "approved" } } });
    await waitFor(() => expect(screen.getByText("Add exception")).toBeEnabled());
    fireEvent.click(screen.getByText("Add exception"));
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Instead of blocking" }));
    fireEvent.click(within(await screen.findByRole("listbox")).getByText("Log and raise event"));
    fireEvent.change(screen.getByTestId("gx-justification"), { target: { value: "PR fixtures" } });
    expect(screen.getByTestId("gx-save")).toBeDisabled();
    fireEvent.change(screen.getByTestId("gx-alert-event"), { target: { value: "Bad Name" } });
    expect(screen.getByTestId("gx-save")).toBeDisabled();
    fireEvent.change(screen.getByTestId("gx-alert-event"), { target: { value: "visor-pr-flag-2" } });
    fireEvent.click(screen.getByTestId("gx-save"));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith(
        "/apps/7/guardrail-exceptions",
        expect.objectContaining({ action: "alert", alert_event: "visor-pr-flag-2" })
      )
    );
  });
});

