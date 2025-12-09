import apiClient from '../utils/apiClient';

class ScheduleService {
  async getPluginSchedules(pluginId) {
    const response = await apiClient.get(`/plugins/${pluginId}/schedules`);
    return response.data;
  }

  async getScheduleDetail(pluginId, scheduleId) {
    const response = await apiClient.get(`/plugins/${pluginId}/schedules/${scheduleId}`);
    return response.data;
  }

  async getScheduleExecutions(pluginId, scheduleId, limit = 50, offset = 0) {
    const response = await apiClient.get(
      `/plugins/${pluginId}/schedules/${scheduleId}/executions`,
      { params: { limit, offset } }
    );
    return response.data;
  }

  async updateSchedule(pluginId, scheduleId, updates) {
    const response = await apiClient.put(
      `/plugins/${pluginId}/schedules/${scheduleId}`,
      updates
    );
    return response.data;
  }

  async deleteSchedule(pluginId, scheduleId) {
    const response = await apiClient.delete(
      `/plugins/${pluginId}/schedules/${scheduleId}`
    );
    return response.data;
  }
}

export default new ScheduleService();
