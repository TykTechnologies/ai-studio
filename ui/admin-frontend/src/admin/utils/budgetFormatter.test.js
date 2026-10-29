import { formatBudgetDisplay } from "./budgetFormatter";

describe("formatBudgetDisplay currency", () => {
  it("prints dollars when the spend has no currency or USD", () => {
    expect(formatBudgetDisplay({ budget: 10, spent: 2.5 })).toMatch(/^\$10\.00 \(25\.0% used, \$2\.50\) for /);
    expect(formatBudgetDisplay({ budget: 10, spent: 2.5, currency: "USD" })).toMatch(/^\$10\.00 /);
  });

  it("prints the budget in the currency its spend implies", () => {
    expect(formatBudgetDisplay({ budget: 10, spent: 2.5, currency: "EUR" })).toMatch(/^EUR 10\.00 \(25\.0% used, EUR 2\.50\) for /);
  });
});

describe("formatBudgetDisplay period", () => {
  it("shows the period the API computed: Feb 28 to Mar 30 for an anchor on the 31st", () => {
    const text = formatBudgetDisplay({
      budget: 10, spent: 1, budgetStartDate: "2026-01-31T00:00:00Z",
      period_start: "2026-02-28T00:00:00Z", period_end: "2026-03-31T00:00:00Z",
    });
    expect(text).toMatch(/ for Feb 28 - Mar 30$/);
  });

  it("reads the server's calendar date whatever the offset", () => {
    const text = formatBudgetDisplay({
      budget: 10, spent: 1, periodStart: "2026-10-24T00:00:00+12:00", periodEnd: "2026-11-24T00:00:00+12:00",
    });
    expect(text).toMatch(/ for Oct 24 - Nov 23$/);
  });
});
