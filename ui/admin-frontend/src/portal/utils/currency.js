// A budget's currency is implied by its spend: none or USD prints as
// "$x.xx", another currency as "EUR x.xx".
export const formatMoney = (value, currency) => {
  if (typeof value !== "number" || Number.isNaN(value)) return "—";
  return currency && currency !== "USD" ? `${currency} ${value.toFixed(2)}` : `$${value.toFixed(2)}`;
};

// "EUR 2.00, USD 1.00" for spend by currency (a period that mixes
// currencies, which a budget refuses).
export const formatCurrencySpend = (spend) =>
  Object.entries(spend || {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([currency, amount]) => `${currency} ${Number(amount).toFixed(2)}`)
    .join(", ");
