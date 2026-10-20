import {
  mapSummary,
  mapTimeseries,
  mapToolRow,
  mapAppRow,
  mapServerRow,
  mapOverview,
  intervalFor,
  fillBuckets,
  fetchServerTools,
  fetchAppServers,
  adminAppPath,
  portalAppPath,
  callsChartData,
  formatPercent,
  formatMs,
} from "./mcpAnalytics";

describe("mcpAnalytics mappers", () => {
  it("maps a summary and derives the error rate when the API omits it", () => {
    expect(mapSummary({ calls: 200, errors: 10, p50_ms: 12, p95_ms: 80, last_seen_at: "t" })).toEqual({
      calls: 200,
      errors: 10,
      errorRate: 0.05,
      avgMs: 0,
      p50Ms: 12,
      p95Ms: 80,
      lastSeenAt: "t",
    });
    expect(mapSummary({ calls: 4, errors: 1, error_rate: 0.3 }).errorRate).toBe(0.3);
    expect(mapSummary(null)).toMatchObject({ calls: 0, errors: 0, errorRate: 0, lastSeenAt: null });
  });

  it("maps timeseries points", () => {
    expect(mapTimeseries({ points: [{ bucket: "2026-10-20T00:00:00Z", calls: 3, errors: 1, p95_ms: 9 }] })).toEqual([
      { bucket: "2026-10-20T00:00:00Z", calls: 3, errors: 1, p50Ms: 0, p95Ms: 9 },
    ]);
    expect(mapTimeseries(undefined)).toEqual([]);
  });

  it("keys tool rows by type and name", () => {
    expect(mapToolRow({ primitive_type: "tool", primitive_name: "search", calls: 2 })).toMatchObject({
      id: "tool:search",
      name: "search",
      primitiveType: "tool",
    });
    expect(mapToolRow({ primitive_type: "discovery" }).name).toBe("(none)");
  });

  it("labels the unattributed App bucket", () => {
    expect(mapAppRow({ app_id: null, calls: 5 })).toMatchObject({ id: "unattributed", appId: null, name: "Unattributed" });
    expect(mapAppRow({ app_id: 3, app_name: "Bot" })).toMatchObject({ id: 3, appId: 3, name: "Bot" });
  });

  it("maps server rows and the overview", () => {
    expect(mapServerRow({ mcp_server_id: 4, server_name: "Weather", connection_id: 7, calls: 1 })).toMatchObject({
      id: 4,
      name: "Weather",
      connectionId: 7,
    });
    const o = mapOverview({ calls: 10, errors: 1, top_tools: [{ mcp_server_id: 4, primitive_name: "x", calls: 6 }] });
    expect(o.errorRate).toBeCloseTo(0.1);
    expect(o.topTools[0]).toMatchObject({ id: "4:x", serverName: "MCP server 4", calls: 6 });
  });

  it("fills every bucket of the range, keeping the API's points", () => {
    const pts = [{ bucket: "2026-10-19T00:00:00Z", calls: 5, errors: 1, p50Ms: 2, p95Ms: 7 }];
    const days = fillBuckets(pts, { startDate: "2026-10-18", endDate: "2026-10-20" }, "day");
    expect(days.map((p) => p.calls)).toEqual([0, 5, 0]);
    expect(days[1]).toBe(pts[0]);
    expect(days[0].p95Ms).toBeNull();

    const hours = fillBuckets([{ bucket: "2026-10-20T09:00:00Z", calls: 2, errors: 0 }], { startDate: "2026-10-20", endDate: "2026-10-20" }, "hour");
    expect(hours).toHaveLength(24);
    expect(hours[9].calls).toBe(2);

    // An off-boundary point is kept, not dropped.
    const odd = fillBuckets([{ bucket: "2026-10-19T05:30:00Z", calls: 1 }], { startDate: "2026-10-19", endDate: "2026-10-19" }, "day");
    expect(odd.map((p) => p.calls)).toEqual([0, 1]);
  });

  it("picks hourly buckets for short ranges", () => {
    expect(intervalFor("2026-10-19", "2026-10-20")).toBe("hour");
    expect(intervalFor("2026-09-20", "2026-10-20")).toBe("day");
  });

  it("formats rates and latency", () => {
    expect(formatPercent(0.05)).toBe("5.0%");
    expect(formatPercent(0)).toBe("0.0%");
    expect(formatMs(250)).toBe("250 ms");
    expect(formatMs(1500)).toBe("1.50 s");
  });

  it("builds calls and errors datasets", () => {
    const d = callsChartData([{ bucket: "b", calls: 2, errors: 1 }]);
    expect(d.datasets.map((s) => s.label)).toEqual(["Calls", "Errors"]);
    expect(d.datasets[1].data).toEqual([{ x: "b", y: 1 }]);
  });
});

describe("mcpAnalytics fetchers", () => {
  it("passes the range and list query to the tools endpoint", async () => {
    const client = { get: jest.fn().mockResolvedValue({ data: { data: [{ primitive_name: "a" }], total: 11, total_pages: 2 } }) };
    const res = await fetchServerTools(client, 9, { startDate: "2026-10-01", endDate: "2026-10-20" }, { page: 2, page_size: 10, sort: "-calls" });
    expect(client.get).toHaveBeenCalledWith("/analytics/mcp/servers/9/tools", {
      params: { start_date: "2026-10-01", end_date: "2026-10-20", page: 2, page_size: 10, sort: "-calls" },
    });
    expect(res.total).toBe(11);
    expect(res.totalPages).toBe(2);
    expect(res.rows[0].name).toBe("a");
  });

  it("uses the portal path for portal App usage", async () => {
    const client = { get: jest.fn().mockResolvedValue({ data: { servers: [{ mcp_server_id: 1, calls: 3 }] } }) };
    const rows = await fetchAppServers(client, portalAppPath(5), { startDate: "a", endDate: "b" });
    expect(client.get).toHaveBeenCalledWith("/common/apps/5/analytics/mcp", { params: { start_date: "a", end_date: "b" } });
    expect(rows).toHaveLength(1);
    expect(adminAppPath(5)).toBe("/analytics/mcp/apps/5");
  });
});
