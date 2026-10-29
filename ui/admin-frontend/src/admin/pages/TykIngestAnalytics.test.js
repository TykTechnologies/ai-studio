import React from "react";
import fs from "fs";
import path from "path";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../utils/testTheme";
import TykIngestAnalytics, {
  httpPumpSnippet,
  noDataWarning,
  vectorSnippet,
  TOKEN_PLACEHOLDER,
} from "./TykIngestAnalytics";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

const STATUS_PATH = "/tyk-connections/7/ingest-status";
const TOKEN_PATH = "/tyk-connections/7/ingest-token";
const recent = () => new Date(Date.now() - 5 * 60 * 1000).toISOString();
const old = () => new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();

const renderSection = (props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <TykIngestAnalytics connectionId="7" canWrite notify={jest.fn()} {...props} />
    </ThemeProvider>,
  );

let serverStatus;

describe("TykIngestAnalytics", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    serverStatus = { has_token: false };
    apiClient.get.mockImplementation((path) => {
      if (path === STATUS_PATH) return Promise.resolve({ data: serverStatus });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockImplementation(() => {
      serverStatus = { has_token: true, token_created_at: new Date().toISOString(), records_24h: 0, unknown_api_24h: 0 };
      return Promise.resolve({ data: { token: "tok-plain-123", status: serverStatus } });
    });
    apiClient.delete.mockImplementation(() => {
      serverStatus = { has_token: false };
      return Promise.resolve({ data: {} });
    });
  });

  it("generates a token, shows the plaintext once and embeds it only in the http pump snippet", async () => {
    renderSection();
    fireEvent.click(await screen.findByTestId("ingest-generate"));

    expect(await screen.findByTestId("ingest-plaintext-value")).toHaveTextContent("tok-plain-123");
    expect(apiClient.post).toHaveBeenCalledWith(TOKEN_PATH, {});
    expect(screen.getByTestId("ingest-rotate")).toBeInTheDocument();
    expect(screen.getByTestId("snippet-http-pump")).toHaveTextContent("Bearer tok-plain-123");
    expect(screen.getByTestId("snippet-vector")).not.toHaveTextContent("tok-plain-123");
    expect(screen.getByTestId("snippet-vector")).toHaveTextContent('auth.token = "${MCP_INGEST_TOKEN}"');

    // Dismissing drops the plaintext; nothing can fetch it back.
    fireEvent.click(within(screen.getByTestId("ingest-plaintext")).getByTitle("Close"));
    await waitFor(() => expect(screen.queryByTestId("ingest-plaintext")).not.toBeInTheDocument());
    expect(screen.getByTestId("snippet-http-pump")).toHaveTextContent(`Bearer ${TOKEN_PLACEHOLDER}`);
  });

  it("never shows a plaintext after a reload", async () => {
    serverStatus = {
      has_token: true,
      token_prefix: "tai_ing_ab12",
      token_created_at: recent(),
      last_batch_at: recent(),
      records_24h: 1234,
      unknown_api_24h: 2,
      unattributed_24h: 5,
      last_unknown_api_id: "api-x",
      ingest_path: "/api/v1/ingest/tyk-analytics",
    };
    renderSection();
    expect(await screen.findByTestId("ingest-records-24h")).toHaveTextContent("1,234");
    expect(screen.getByTestId("ingest-unknown-api")).toHaveTextContent("2");
    expect(screen.getByTestId("ingest-unattributed")).toHaveTextContent("5");
    expect(screen.getByTestId("ingest-token-state")).toHaveTextContent("tai_ing_ab12…");
    expect(screen.getByTestId("ingest-unknown-api-hint")).toHaveTextContent("api-x");
    expect(screen.queryByTestId("ingest-plaintext")).not.toBeInTheDocument();
    expect(screen.getByTestId("snippet-http-pump")).toHaveTextContent(`Bearer ${TOKEN_PLACEHOLDER}`);
    expect(screen.queryByTestId("ingest-no-data")).not.toBeInTheDocument();
  });

  it("rotates only after confirmation", async () => {
    serverStatus = { has_token: true, token_created_at: old(), last_batch_at: recent() };
    renderSection();
    fireEvent.click(await screen.findByTestId("ingest-rotate"));
    const dialog = await screen.findByTestId("ingest-rotate-dialog");
    expect(apiClient.post).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByText("Rotate token"));
    expect(await screen.findByTestId("ingest-plaintext-value")).toHaveTextContent("tok-plain-123");
    expect(apiClient.post).toHaveBeenCalledWith(TOKEN_PATH, {});
  });

  it("revokes after confirmation and refreshes the status", async () => {
    serverStatus = { has_token: true, token_created_at: old(), last_batch_at: recent() };
    renderSection();
    fireEvent.click(await screen.findByTestId("ingest-revoke"));
    const dialog = await screen.findByTestId("ingest-revoke-dialog");
    fireEvent.click(within(dialog).getByText("Cancel"));
    expect(apiClient.delete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("ingest-revoke"));
    fireEvent.click(within(await screen.findByTestId("ingest-revoke-dialog")).getByText("Revoke"));
    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith(TOKEN_PATH));
    expect(await screen.findByTestId("ingest-generate")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledTimes(2);
  });

  it("warns when a token exists but nothing arrived for an hour", async () => {
    serverStatus = { has_token: true, token_created_at: old(), last_batch_at: old(), records_24h: 0 };
    renderSection();
    expect(await screen.findByTestId("ingest-no-data")).toHaveTextContent("over an hour");
  });

  it("hides token actions without write permission", async () => {
    serverStatus = { has_token: true, last_batch_at: recent() };
    renderSection({ canWrite: false });
    await screen.findByTestId("ingest-status");
    expect(screen.queryByTestId("ingest-rotate")).not.toBeInTheDocument();
    expect(screen.queryByTestId("ingest-revoke")).not.toBeInTheDocument();
  });

  it("shows the API error when the status cannot be loaded", async () => {
    apiClient.get.mockRejectedValue({ response: { data: { errors: [{ detail: "boom" }] } } });
    renderSection();
    expect(await screen.findByTestId("ingest-error")).toHaveTextContent("boom");
  });
});

describe("snippets", () => {
  it("builds the http pump block with the ingest URL", () => {
    const conf = JSON.parse(httpPumpSnippet("https://studio.example.com/api/v1/ingest/tyk-analytics", null));
    const pump = conf.pumps.http;
    expect(pump.type).toBe("http");
    expect(pump.timeout).toBe(30);
    expect(pump.meta).not.toHaveProperty("timeout");
    expect(pump.meta.max_retries).toBe(8);
    expect(conf.dont_purge_uptime_data).toBe(true);
    expect(pump.meta.gzip).toBe(true);
    expect(pump.omit_detailed_recording).toBe(true);
    expect(pump.meta.url).toBe("https://studio.example.com/api/v1/ingest/tyk-analytics");
    expect(pump.meta.headers.Authorization).toBe(`Bearer ${TOKEN_PLACEHOLDER}`);
    expect(pump.meta.mcp_only).toBe(true);
  });

  it("points Vector at the ingest URL", () => {
    const v = vectorSnippet("https://s/api/v1/ingest/tyk-analytics");
    expect(v).toContain("#   MCP_INGEST_URL    https://s/api/v1/ingest/tyk-analytics");
    expect(v).toContain('uri = "${MCP_INGEST_URL}"');
    expect(v).toContain('auth.strategy = "bearer"');
    expect(v).toContain("if record.mcp_stats.is_mcp != true { abort }");
  });
});

describe("vector snippet drift", () => {
  it("matches the Vector config tested end to end", () => {
    const tested = fs.readFileSync(path.resolve(__dirname, "../../../../../tests/tykmcp/oss/vector.toml"), "utf8");
    const body = (s) => s.slice(s.indexOf("# Pump's container output")).trim();
    expect(body(vectorSnippet("https://s/api/v1/ingest/tyk-analytics"))).toBe(body(tested));
  });
});

describe("noDataWarning", () => {
  const now = Date.parse("2026-10-20T12:00:00Z");
  it("is off without a token", () => expect(noDataWarning({ has_token: false }, now)).toBe(false));
  it("is off for a fresh token without data", () =>
    expect(noDataWarning({ has_token: true, token_created_at: "2026-10-20T11:30:00Z" }, now)).toBe(false));
  it("is on for an hour-old token without data", () =>
    expect(noDataWarning({ has_token: true, token_created_at: "2026-10-20T10:30:00Z" }, now)).toBe(true));
  it("is on when the last batch is over an hour old", () =>
    expect(noDataWarning({ has_token: true, last_batch_at: "2026-10-20T10:00:00Z" }, now)).toBe(true));
  it("is off with a recent batch", () =>
    expect(noDataWarning({ has_token: true, last_batch_at: "2026-10-20T11:50:00Z" }, now)).toBe(false));
});
