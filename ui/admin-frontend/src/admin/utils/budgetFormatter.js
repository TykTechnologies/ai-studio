// Get current date (allows for mocking in tests)
export const getNow = () => new Date();

// Calculate the start of the current budget period based on a reference date
const calculateBudgetPeriodStart = (referenceDate) => {
	if (!referenceDate) {
		// If no reference date, use 1st of current month
		const now = getNow();
		return new Date(now.getFullYear(), now.getMonth(), 1);
	}

	// Use the reference date directly
	return new Date(referenceDate);
};

// Format the budget period as a string (e.g., "Jan 14 - Feb 13")
const formatBudgetPeriod = (startDate) => {
	if (!startDate) {
		const now = getNow();
		const lastDay = new Date(now.getFullYear(), now.getMonth() + 1, 0);
		return `${now.toLocaleString('default', { month: 'short' })} 1 - ${now.toLocaleString('default', { month: 'short' })} ${lastDay.getDate()}`;
	}

	const start = new Date(startDate);
	const end = new Date(startDate);
	end.setMonth(end.getMonth() + 1);
	end.setDate(end.getDate() - 1);

	return `${start.toLocaleString('default', { month: 'short' })} ${start.getDate()} - ${end.toLocaleString('default', { month: 'short' })} ${end.getDate()}`;
};

export const formatBudgetDisplay = (item) => {
	if (!item.monthlyBudget && !item.attributes?.monthly_budget) {
		return "not set";
	}

	const monthlyBudget = item.monthlyBudget || item.attributes?.monthly_budget;
	const usagePercent = item.usagePercent || item.percentage || 0;
	const currentUsage = item.currentUsage || item.current_usage || 0;
	const budgetStartDate = item.budgetStartDate || item.start_date || item.attributes?.budget_start_date;

	const periodStart = calculateBudgetPeriodStart(budgetStartDate);
	const period = formatBudgetPeriod(periodStart);

	return `$${parseFloat(monthlyBudget).toFixed(2)} (${parseFloat(usagePercent).toFixed(1)}% used, $${parseFloat(currentUsage).toFixed(2)}) for ${period}`;
};
