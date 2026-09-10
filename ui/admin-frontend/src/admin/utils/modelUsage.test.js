import {
  parseUsageTime,
  formatUsageTime,
  modelDetailPath,
  sortUsageRows,
  nextSortConfig,
} from "./modelUsage";

describe("parseUsageTime", () => {
  it("parses ISO timestamps from the analytics API", () => {
    expect(parseUsageTime("2026-09-08T10:11:12Z")?.toISOString()).toBe("2026-09-08T10:11:12.000Z");
  });

  it("treats Go's zero time and empty values as missing", () => {
    expect(parseUsageTime("0001-01-01T00:00:00Z")).toBeNull();
    expect(parseUsageTime("")).toBeNull();
    expect(parseUsageTime(null)).toBeNull();
    expect(parseUsageTime("not a date")).toBeNull();
  });
});

describe("formatUsageTime", () => {
  const now = new Date("2026-09-11T12:00:00Z");

  it("renders a relative and an absolute form", () => {
    const out = formatUsageTime("2026-09-07T12:00:00Z", now);
    expect(out.relative).toBe("4 days ago");
    // Absolute form is in the viewer's local time, so only check the shape.
    expect(out.absolute).toMatch(/^\d{1,2} Sep 2026 \d{2}:\d{2}$/);
  });

  it("says just now for very recent calls", () => {
    expect(formatUsageTime("2026-09-11T11:59:40Z", now).relative).toBe("Just now");
  });

  it("says never when there is no timestamp", () => {
    expect(formatUsageTime(null, now)).toEqual({ relative: "Never", absolute: "" });
  });
});

describe("modelDetailPath", () => {
  it("keeps vendor model IDs with colons and slashes intact in the query string", () => {
    expect(modelDetailPath(7, "anthropic.claude-3-5-sonnet-20241022-v2:0")).toBe(
      "/admin/llms/7/models?model=anthropic.claude-3-5-sonnet-20241022-v2%3A0",
    );
    expect(modelDetailPath("3", "meta-llama/llama-3-70b")).toBe(
      "/admin/llms/3/models?model=meta-llama%2Fllama-3-70b",
    );
  });
});

describe("sortUsageRows", () => {
  const columns = { model: "string", requestCount: "number", lastUsed: "date" };
  const rows = [
    { model: "gpt-4o", requestCount: 10, lastUsed: "2026-09-10T00:00:00Z" },
    { model: "claude-3", requestCount: 300, lastUsed: "2026-09-01T00:00:00Z" },
    { model: "Gemini", requestCount: 5, lastUsed: null },
  ];

  it("sorts dates newest first and puts missing dates last", () => {
    const out = sortUsageRows(rows, { field: "lastUsed", direction: "desc" }, columns);
    expect(out.map((r) => r.model)).toEqual(["gpt-4o", "claude-3", "Gemini"]);
  });

  it("sorts numbers numerically", () => {
    const out = sortUsageRows(rows, { field: "requestCount", direction: "asc" }, columns);
    expect(out.map((r) => r.requestCount)).toEqual([5, 10, 300]);
  });

  it("sorts strings case-insensitively", () => {
    const out = sortUsageRows(rows, { field: "model", direction: "asc" }, columns);
    expect(out.map((r) => r.model)).toEqual(["claude-3", "Gemini", "gpt-4o"]);
  });

  it("does not mutate the input", () => {
    const copy = [...rows];
    sortUsageRows(rows, { field: "requestCount", direction: "desc" }, columns);
    expect(rows).toEqual(copy);
  });

  it("handles a non-array gracefully", () => {
    expect(sortUsageRows(undefined, { field: "model", direction: "asc" }, columns)).toEqual([]);
  });
});

describe("nextSortConfig", () => {
  const columns = { model: "string", requestCount: "number" };

  it("starts numeric columns descending and text columns ascending", () => {
    expect(nextSortConfig({ field: "model", direction: "asc" }, "requestCount", columns)).toEqual({
      field: "requestCount",
      direction: "desc",
    });
    expect(nextSortConfig({ field: "requestCount", direction: "desc" }, "model", columns)).toEqual({
      field: "model",
      direction: "asc",
    });
  });

  it("flips direction when the same column is clicked again", () => {
    expect(nextSortConfig({ field: "model", direction: "asc" }, "model", columns)).toEqual({
      field: "model",
      direction: "desc",
    });
  });
});
