import apiClient from "../utils/apiClient";

// Per-member App quota of self-service creation (Enterprise): how many Apps
// each member of a team may create through the portal. A user in several
// teams gets the most generous; without one, the global default applies.
export const appQuotaService = {
  // { group_id, set, max_apps_per_member, unlimited, default_max, default_unlimited }
  getTeamQuota: async (teamId) => {
    const response = await apiClient.get(`/groups/${teamId}/app-quota`);
    return response.data;
  },

  // quota: { max_apps_per_member } or { unlimited: true }
  setTeamQuota: async (teamId, quota) => {
    const response = await apiClient.put(`/groups/${teamId}/app-quota`, quota);
    return response.data;
  },

  removeTeamQuota: async (teamId) => {
    await apiClient.delete(`/groups/${teamId}/app-quota`);
  },
};
