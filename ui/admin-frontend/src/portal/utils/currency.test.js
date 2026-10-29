import { formatMoney, formatCurrencySpend } from "./currency";

describe("formatMoney", () => {
  it("prints dollars for USD or no currency, and the code otherwise", () => {
    expect(formatMoney(2.5)).toBe("$2.50");
    expect(formatMoney(2.5, "USD")).toBe("$2.50");
    expect(formatMoney(2.5, "EUR")).toBe("EUR 2.50");
    expect(formatMoney(undefined, "EUR")).toBe("—");
  });
});

describe("formatCurrencySpend", () => {
  it("lists spend by currency, sorted", () => {
    expect(formatCurrencySpend({ USD: 18.74, EUR: 58.32 })).toBe("EUR 58.32, USD 18.74");
    expect(formatCurrencySpend(undefined)).toBe("");
  });
});
