// Get current date (allows for mocking in tests)
export const getNow = () => new Date();

// Midnight UTC on day of the month (month may run past either end of the
// year), or on the month's last day when it has fewer days.
const anchorIn = (year, month, day) => {
	const last = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
	return new Date(Date.UTC(year, month, Math.min(day, last)));
};

// The current budget period, for callers without the one the API computed:
// the same rule as services/budget.Period (monthly from the reference date's
// day, the 1st when there is none, anchor days 29-31 clamped to a short
// month's last day), in UTC. Returns [start, next period's start].
const estimateBudgetPeriod = (referenceDate) => {
	const now = getNow();
	const day = referenceDate ? new Date(referenceDate).getUTCDate() : 1;
	let start = anchorIn(now.getUTCFullYear(), now.getUTCMonth(), day);
	if (now < start) {
		start = anchorIn(now.getUTCFullYear(), now.getUTCMonth() - 1, day);
	}
	return [start, anchorIn(start.getUTCFullYear(), start.getUTCMonth() + 1, day)];
};

// "Feb 28 - Mar 30" for a period from start to (excluding) next, both
// midnight UTC.
const formatPeriod = (start, next) => {
	const last = new Date(next.getTime());
	last.setUTCDate(last.getUTCDate() - 1);
	const label = (date) => `${date.toLocaleString("default", { month: "short", timeZone: "UTC" })} ${date.getUTCDate()}`;
	return `${label(start)} - ${label(last)}`;
};

// The period the API computed (as enforcement does): periodStart is its
// first day and periodEnd the next period's start. The calendar date is read
// from the timestamp as the server wrote it, so the browser's time zone
// cannot shift it.
const formatApiPeriod = (periodStart, periodEnd) => {
	const day = (iso) => {
		const [y, m, d] = String(iso).slice(0, 10).split("-").map(Number);
		return new Date(Date.UTC(y, m - 1, d));
	};
	return formatPeriod(day(periodStart), day(periodEnd));
};

// A budget of null (or undefined) is "no limit"; 0 is a budget of zero.
export const formatBudgetDisplay = (item) => {
	const monthlyBudget = item.budget ?? item.monthlyBudget;
	const budgetStartDate = item.budgetStartDate;
	const spent = item.spent || item.currentUsage || 0;

	if (monthlyBudget === null || monthlyBudget === undefined) {
		return "No limit";
	}
	if (Number(monthlyBudget) === 0) {
		return "$0.00 (nothing may be spent; requests are refused)";
	}

	const usagePercent = (spent / monthlyBudget) * 100 || 0;
	const currentUsage = spent;

	// The period the API computed when the caller has it; otherwise the
	// same rule worked out here.
	const periodStart = item.periodStart ?? item.period_start;
	const periodEnd = item.periodEnd ?? item.period_end;
	const period = periodStart && periodEnd
		? formatApiPeriod(periodStart, periodEnd)
		: formatPeriod(...estimateBudgetPeriod(budgetStartDate));

	// A budget's currency is implied by its spend (item.currency); none or
	// USD shows as dollars.
	const money = (v) => {
		const amount = parseFloat(v).toFixed(2);
		return item.currency && item.currency !== "USD" ? `${item.currency} ${amount}` : `$${amount}`;
	};
	return `${money(monthlyBudget)} (${parseFloat(usagePercent).toFixed(1)}% used, ${money(currentUsage)}) for ${period}`;
};
