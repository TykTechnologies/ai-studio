import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import AuditTrail, { DiffTable } from "./AuditTrail";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, ...props }) => <div {...props}>{children}</div>,
  StyledTableHeaderCell: ({ children, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, sx, ...props }) => <td {...props}>{children}</td>,
  // Strip MUI-only props so the plain <tr> does not warn about unknown attributes.
  StyledTableRow: ({ children, hover, sx, ...props }) => <tr {...props}>{children}</tr>,
}));

jest.mock("../components/common/DateRangePicker", () => () => (
  <div data-testid="date-range-picker" />
));

const theme = createTheme();

const renderPage = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <AuditTrail />
      </MemoryRouter>
    </ThemeProvider>
  );

const enabledStatus = {
  available: true,
  enabled: true,
  store_type: "db",
  detailed_recording: false,
  record_reads: false,
  retention_days: 90,
  queue_depth: 0,
  dropped: 0,
};

const records = [
  {
    id: 1,
    req_id: "req-1",
    timestamp: "2026-09-10T10:00:00Z",
    ip: "10.0.0.1",
    user_id: 1,
    user: "admin@tyk.io",
    action: "Update LLM",
    method: "PATCH",
    url: "/api/v1/llms/1",
    route: "/api/v1/llms/:id",
    status: 200,
    resource_type: "llm",
    resource_id: "1",
    resource_name: "gpt-4",
    diff: { name: { old: "gpt-4", new: "gpt-4-turbo" }, api_key: { old: "[REDACTED]", new: "[REDACTED]" } },
    duration_ms: 12,
  },
  {
    id: 2,
    req_id: "req-2",
    timestamp: "2026-09-10T11:00:00Z",
    ip: "10.0.0.9",
    user_id: 0,
    user: "intruder@evil.io",
    action: "Login Failed",
    method: "POST",
    url: "/auth/login",
    route: "/auth/login",
    status: 401,
    resource_type: "session",
    error: "Invalid email or password",
    duration_ms: 3,
  },
];

const summary = {
  total: 2,
  failed: 1,
  distinct_users: 2,
  by_action: [
    { name: "Update LLM", count: 1 },
    { name: "Login Failed", count: 1 },
  ],
  by_user: [],
  by_resource_type: [{ name: "llm", count: 1 }],
  by_status_class: [],
  timeline: [],
};

const mockApi = ({ status = enabledStatus } = {}) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/audit/status") return Promise.resolve({ data: status });
    if (url === "/audit/records") return Promise.resolve({ data: { records, total: 2, page: 1, page_size: 25 } });
    if (url === "/audit/summary") return Promise.resolve({ data: summary });
    if (url === "/audit/records/1") {
      return Promise.resolve({
        data: { ...records[0], request_dump: { headers: { "Content-Type": "application/json" } } },
      });
    }
    return Promise.reject(new Error(`unexpected ${url}`));
  });
};

beforeEach(() => {
  jest.clearAllMocks();
});

describe("AuditTrail", () => {
  it("shows the enterprise upsell when the feature is unavailable", async () => {
    mockApi({ status: { available: false } });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/audit/status");
    expect(apiClient.get).not.toHaveBeenCalledWith("/audit/records", expect.anything());
  });

  it("explains when recording is switched off", async () => {
    mockApi({ status: { ...enabledStatus, enabled: false } });
    renderPage();
    expect(await screen.findByText(/audit trail is switched off/i)).toBeInTheDocument();
  });

  it("explains when only the file store is configured", async () => {
    mockApi({ status: { ...enabledStatus, store_type: "file" } });
    renderPage();
    expect(await screen.findByText(/write to a file only/i)).toBeInTheDocument();
  });

  it("loads records and summary with the default date range", async () => {
    mockApi();
    renderPage();

    expect(await screen.findByText("Update LLM")).toBeInTheDocument();
    expect(screen.getByText("Login Failed")).toBeInTheDocument();
    expect(screen.getByText("admin@tyk.io")).toBeInTheDocument();
    expect(screen.getByText("gpt-4")).toBeInTheDocument();
    expect(screen.getByText("401")).toBeInTheDocument();

    const recordsCall = apiClient.get.mock.calls.find((c) => c[0] === "/audit/records");
    expect(recordsCall[1].params).toEqual(
      expect.objectContaining({
        start_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
        end_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
        page: 1,
        page_size: 25,
      })
    );
    expect(recordsCall[1].params.user).toBeUndefined();

    // Summary tiles
    expect(screen.getByText("Recorded actions")).toBeInTheDocument();
    expect(screen.getByText("Failed (4xx/5xx)")).toBeInTheDocument();
    expect(screen.getByText("90d")).toBeInTheDocument();
  });

  it("expands a row, fetches the full record and renders the diff", async () => {
    mockApi();
    renderPage();
    await screen.findByText("Update LLM");

    fireEvent.click(screen.getByTestId("audit-row-1"));

    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/audit/records/1"));
    expect(await screen.findByTestId("audit-diff")).toBeInTheDocument();
    expect(screen.getByText("gpt-4-turbo")).toBeInTheDocument();
    expect(screen.getAllByText("[REDACTED]").length).toBe(2);
    expect(screen.getByText("req-1")).toBeInTheDocument();
    expect(await screen.findByText("Request")).toBeInTheDocument();
  });

  it("applies filters and resets to the first page", async () => {
    mockApi();
    renderPage();
    await screen.findByText("Update LLM");

    fireEvent.change(screen.getByLabelText("User"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Search"), { target: { value: "llms" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));

    await waitFor(() => {
      const calls = apiClient.get.mock.calls.filter((c) => c[0] === "/audit/records");
      const last = calls[calls.length - 1];
      expect(last[1].params).toEqual(
        expect.objectContaining({ user: "admin", search: "llms", page: 1 })
      );
    });
  });

  it("warns when records were dropped", async () => {
    mockApi({ status: { ...enabledStatus, dropped: 7 } });
    renderPage();
    expect(await screen.findByText(/7 audit record\(s\) were dropped/)).toBeInTheDocument();
  });

  it("falls back to the upsell when the API returns 403", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/audit/status") return Promise.resolve({ data: enabledStatus });
      return Promise.reject({ response: { status: 403 } });
    });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
  });
});

describe("DiffTable", () => {
  it("renders nothing for an empty diff", () => {
    const { container } = render(<DiffTable diff={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("lists changed fields and truncated ones", () => {
    render(
      <DiffTable
        diff={{
          name: { old: "a", new: "b" },
          config: { old: { x: 1 }, new: { x: 2 } },
          _truncated_fields: ["blob"],
        }}
      />
    );
    expect(screen.getByText("name")).toBeInTheDocument();
    expect(screen.getByText("config")).toBeInTheDocument();
    expect(screen.getByText(/Not shown \(too large\): blob/)).toBeInTheDocument();
  });
});
