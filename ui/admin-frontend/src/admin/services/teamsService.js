import apiClient from "../utils/apiClient";

export const teamsService = {
  getTeam: async (id) => {
    try {
      const response = await apiClient.get(`/groups/${id}`);
      return response.data;
    } catch (error) {
      console.error("Error fetching team:", error);
      throw error;
    }
  },
  
  getTeams: async (params) => {
    try {
      const response = await apiClient.get("/groups", { params });
      return response;
    } catch (error) {
      console.error("Error fetching teams:", error);
      throw error;
    }
  },
  
  getTeamUsers: async (id, queryParams) => {
    try {
      const response = await apiClient.get(`/groups/${id}/users`, {
        params: queryParams
      });
      return {
        data: response.data.data,
        totalCount: parseInt(response.headers['x-total-count'] || '0', 10),
        totalPages: parseInt(response.headers['x-total-pages'] || '0', 10)
      };
    } catch (error) {
      console.error("Error fetching team users:", error);
      throw error;
    }
  },
  
  createTeam: async (teamData) => {
    try {
      const response = await apiClient.post("/groups", teamData);
      return response.data;
    } catch (error) {
      console.error("Error creating team:", error);
      throw error;
    }
  },
  
  updateTeam: async (id, teamData) => {
    try {
      const response = await apiClient.patch(`/groups/${id}`, teamData);
      return response.data;
    } catch (error) {
      console.error("Error updating team:", error);
      throw error;
    }
  },
  
  deleteTeam: async (id) => {
    try {
      await apiClient.delete(`/groups/${id}`);
    } catch (error) {
      console.error("Error deleting team:", error);
      throw error;
    }
  },

  updateGroupUsers: async (id, userIds) => {
    try {
      const response = await apiClient.put(`/groups/${id}/users`, {
        data: {
          type: "groups",
          attributes: {
            members: userIds
          }
        }
      });
      return response.data;
    } catch (error) {
      console.error("Error updating team users:", error);
      throw error;
    }
  }
};