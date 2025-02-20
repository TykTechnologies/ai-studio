// Get current date (allows for mocking in tests)
export const getNow = () => new Date();

// Calculate the start of the current budget period based on a reference date
const calculateBudgetPeriodStart = (referenceDate) => {
	if (!referenceDate) {
		// If no reference date, use 1st of current month
		const now = getNow();
		return new Date(Date.UTC(now.getFullYear(), now.getMonth(), 1));
	}

	// Parse the ISO date string and ensure it's in UTC
	const date = new Date(referenceDate);
	return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
};

// Format the budget period as a string (e.g., "Jan 14 - Feb 13")
const formatBudgetPeriod = (startDate) => {
	if (!startDate) {
		const now = getNow();
		const lastDay = new Date(now.getFullYear(), now.getMonth() + 1, 0);
		return `${now.toLocaleString('default', { month: 'short' })} 1 - ${now.toLocaleString('default', { month: 'short' })} ${lastDay.getDate()}`;
	}

	const start = new Date(startDate);
	const end = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 1, start.getUTCDate()));

	return `${start.toLocaleString('default', { month: 'short' })} ${start.getDate()} - ${end.toLocaleString('default', { month: 'short' })} ${end.getDate()}`;
};

export const formatBudgetDisplay = (item) => {
	const monthlyBudget = item.budget || item.monthlyBudget;
	const budgetStartDate = item.budgetStartDate;
	const spent = item.spent || item.currentUsage || 0;


	if (!monthlyBudget) {
		return "not set";
	}

	const usagePercent = (spent / monthlyBudget) * 100 || 0;
	const currentUsage = spent;

	const periodStart = calculateBudgetPeriodStart(budgetStartDate);
	const period = formatBudgetPeriod(periodStart);

	return `$${parseFloat(monthlyBudget).toFixed(2)} (${parseFloat(usagePercent).toFixed(1)}% used, $${parseFloat(currentUsage).toFixed(2)}) for ${period}`;
};
