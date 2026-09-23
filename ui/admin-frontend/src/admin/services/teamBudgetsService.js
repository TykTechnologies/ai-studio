import apiClient from "../utils/apiClient";

// Team budgets (Enterprise): a monthly ceiling on a team's spend and an
// allocation pool its Apps draw their budgets from.
export const teamBudgetsService = {
  getSettings: async () => {
    const response = await apiClient.get("/team-budgets/settings");
    return response.data;
  },

  setEnabled: async (enabled) => {
    const response = await apiClient.put("/team-budgets/settings", { enabled });
    return response.data;
  },

  // Report: budget, period, spent, allocated/unallocated, per-App rows.
  getReport: async (teamId) => {
    const response = await apiClient.get(`/groups/${teamId}/budget`);
    return response.data;
  },

  setBudget: async (teamId, budget) => {
    const response = await apiClient.put(`/groups/${teamId}/budget`, budget);
    return response.data;
  },

  removeBudget: async (teamId) => {
    await apiClient.delete(`/groups/${teamId}/budget`);
  },

  resetBudget: async (teamId) => {
    await apiClient.post(`/groups/${teamId}/budget/reset`);
  },

  adoptApp: async (appId) => {
    const response = await apiClient.post(`/apps/${appId}/adopt-team-budget`);
    return response.data;
  },

  // Spend per team; dates are YYYY-MM-DD, end inclusive.
  getTeamCosts: async (startDate, endDate) => {
    const response = await apiClient.get("/analytics/team-costs", {
      params: { start_date: startDate, end_date: endDate },
    });
    return response.data;
  },

  setUserBudgetTeam: async (userId, teamId) => {
    await apiClient.put(`/users/${userId}/budget-team`, { team_id: teamId });
  },
};

// errorDetail pulls the API's error message out of an axios error.
export const errorDetail = (error, fallback) =>
  error?.response?.data?.errors?.[0]?.detail || fallback;

export const formatMoney = (value) =>
  value === null || value === undefined ? "—" : `$${Number(value).toFixed(2)}`;
