import { formatBudgetDisplay } from "./budgetFormatter";

// kq3b.16 regression: the budget period the dashboard prints is the one the
// budget is enforced over. Each case is services/budget.Period for an anchor
// (budget start date) and a moment: anchor days 29-31 clamp to a short
// month's last day (Feb 28, Feb 29 in a leap year, Apr 30), mid-month anchors
// start on the anchor day. start is the period's first day and end the next
// period's start, as GET /analytics/budget-usage returns them.
const cases = [
  { anchor: "2025-01-31T00:00:00Z", now: "2026-03-01T12:00:00Z", start: "2026-02-28T00:00:00Z", end: "2026-03-31T00:00:00Z", text: "Feb 28 - Mar 30" },
  { anchor: "2025-01-31T00:00:00Z", now: "2026-03-31T12:00:00Z", start: "2026-03-31T00:00:00Z", end: "2026-04-30T00:00:00Z", text: "Mar 31 - Apr 29" },
  { anchor: "2025-01-31T00:00:00Z", now: "2026-05-15T12:00:00Z", start: "2026-04-30T00:00:00Z", end: "2026-05-31T00:00:00Z", text: "Apr 30 - May 30" },
  { anchor: "2025-01-30T00:00:00Z", now: "2026-03-01T12:00:00Z", start: "2026-02-28T00:00:00Z", end: "2026-03-30T00:00:00Z", text: "Feb 28 - Mar 29" },
  { anchor: "2025-01-29T00:00:00Z", now: "2026-03-01T12:00:00Z", start: "2026-02-28T00:00:00Z", end: "2026-03-29T00:00:00Z", text: "Feb 28 - Mar 28" },
  { anchor: "2025-01-29T00:00:00Z", now: "2028-03-01T12:00:00Z", start: "2028-02-29T00:00:00Z", end: "2028-03-29T00:00:00Z", text: "Feb 29 - Mar 28" },
  { anchor: "2025-01-31T00:00:00Z", now: "2028-03-02T12:00:00Z", start: "2028-02-29T00:00:00Z", end: "2028-03-31T00:00:00Z", text: "Feb 29 - Mar 30" },
  { anchor: "2025-01-15T00:00:00Z", now: "2026-10-29T12:00:00Z", start: "2026-10-15T00:00:00Z", end: "2026-11-15T00:00:00Z", text: "Oct 15 - Nov 14" },
  { anchor: "2025-01-15T00:00:00Z", now: "2026-10-03T12:00:00Z", start: "2026-09-15T00:00:00Z", end: "2026-10-15T00:00:00Z", text: "Sep 15 - Oct 14" },
];

describe("budget period text matches the enforced period (kq3b.16)", () => {
  afterEach(() => jest.useRealTimers());

  it.each(cases)("API period for anchor $anchor at $now", ({ anchor, now, start, end, text }) => {
    jest.useFakeTimers().setSystemTime(new Date(now));
    expect(formatBudgetDisplay({ budget: 10, spent: 1, budgetStartDate: anchor, period_start: start, period_end: end }))
      .toMatch(new RegExp(` for ${text}$`));
    expect(formatBudgetDisplay({ monthlyBudget: 10, currentUsage: 1, budgetStartDate: anchor, periodStart: start, periodEnd: end }))
      .toMatch(new RegExp(` for ${text}$`));
  });
});
