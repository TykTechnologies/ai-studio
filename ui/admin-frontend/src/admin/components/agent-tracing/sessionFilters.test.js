import {
  confidenceShares,
  emptyStateFilters,
  emptyUrlFilters,
  filtersFromSearchParams,
  filtersToSearchParams,
  formatCost,
  formatDuration,
  isSessionUUID,
  sortConfigOf,
  sortParamOf,
  toQueryBody,
} from "./sessionFilters";

const UUID = "0192a4c8-7f00-7000-8000-000000000001";

describe("URL filters", () => {
  it("round-trips every URL filter and leaves defaults out", () => {
    const f = {
      ...emptyUrlFilters(),
      range: "custom",
      from: "2026-10-01",
      to: "2026-10-02",
      app: "7",
      user: "3",
      status: "error",
      source: "inferred",
      weak: true,
      errors: "yes",
      gw: "no",
      costMin: "0.5",
      costMax: "10",
      durMin: "30",
      durMax: "600",
      sort: "-cost",
      session: UUID,
    };
    const sp = filtersToSearchParams(f);
    expect(filtersFromSearchParams(sp)).toEqual(f);
    expect(filtersToSearchParams(emptyUrlFilters()).toString()).toBe("");
  });

  it("drops malformed values from a hand-edited link", () => {
    const sp = new URLSearchParams(
      "range=forever&app=abc&user=-1&status=open&source=x&errors=maybe&cost_min=lots&sort=name&session=alice@example.com",
    );
    expect(filtersFromSearchParams(sp)).toEqual(emptyUrlFilters());
    expect(filtersFromSearchParams(new URLSearchParams("range=custom&from=2026-10-01")).range).toBe("24h");
  });

  it("never writes state-only filters (names, end user, external id) to the URL", () => {
    const sp = filtersToSearchParams({ ...emptyUrlFilters(), agent: "planner", endUser: "alice@example.com", externalId: "conv-1" });
    expect(sp.toString()).toBe("");
  });

  it("accepts only a session UUID as the session filter", () => {
    expect(isSessionUUID(UUID)).toBe(true);
    expect(isSessionUUID(UUID.toUpperCase())).toBe(true);
    expect(isSessionUUID("conv-123")).toBe(false);
  });
});

describe("query body", () => {
  const now = new Date("2026-10-20T12:00:00Z");

  it("defaults to the last 24 hours, newest first, 50 per page", () => {
    expect(toQueryBody(emptyUrlFilters(), emptyStateFilters(), { now })).toEqual({
      sort: "-started_at",
      limit: 50,
      from: "2026-10-19T12:00:00.000Z",
      to: "2026-10-20T12:00:00.000Z",
    });
  });

  it("converts units and carries state filters and the cursor in the body", () => {
    const url = { ...emptyUrlFilters(), range: "1h", app: "7", errors: "no", gw: "yes", weak: true, costMin: "0.25", durMax: "90", session: UUID };
    const state = { ...emptyStateFilters(), agent: " planner ", model: "gpt-4.1", tool: "search", endUser: "alice@example.com", externalId: "conv-1" };
    expect(toQueryBody(url, state, { now, cursor: "abc" })).toEqual({
      sort: "-started_at",
      limit: 50,
      from: "2026-10-20T11:00:00.000Z",
      to: "2026-10-20T12:00:00.000Z",
      app_id: 7,
      has_errors: false,
      has_gateway_events: true,
      weak_links: true,
      cost_min_micros: 250000,
      duration_max_ms: 90000,
      session_id: UUID,
      agent: "planner",
      model: "gpt-4.1",
      tool: "search",
      end_user: "alice@example.com",
      external_id: "conv-1",
      cursor: "abc",
    });
  });

  it("covers whole days for a custom range", () => {
    const body = toQueryBody({ ...emptyUrlFilters(), range: "custom", from: "2026-10-01", to: "2026-10-01" }, emptyStateFilters());
    expect(new Date(body.to) - new Date(body.from)).toBe(24 * 3600e3);
  });
});

describe("formatting", () => {
  it("sorts", () => {
    expect(sortConfigOf("-cost")).toEqual({ field: "cost", direction: "desc" });
    expect(sortConfigOf("")).toEqual({ field: "started_at", direction: "desc" });
    expect(sortParamOf({ field: "tokens", direction: "asc" })).toBe("tokens");
  });

  it("formats durations and costs", () => {
    expect(formatDuration(450)).toBe("450 ms");
    expect(formatDuration(4200)).toBe("4.2 s");
    expect(formatDuration(125000)).toBe("2 min 5 s");
    expect(formatDuration(3 * 3600e3 + 60e3)).toBe("3 h 1 min");
    expect(formatCost(1234567, "USD")).toContain("1.23");
    expect(formatCost(1200, "USD")).toContain("0.0012");
  });

  it("computes the correlation shares", () => {
    const shares = confidenceShares({ declared: 1, exact: 1, strong: 2, weak: 0 });
    expect(shares.map((s) => s.share)).toEqual([0.25, 0.25, 0.5, 0]);
    expect(confidenceShares({}).every((s) => s.share === 0)).toBe(true);
  });
});
