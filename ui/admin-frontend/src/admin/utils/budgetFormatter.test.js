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
