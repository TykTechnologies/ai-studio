import { teamsService } from './teamsService';
import apiClient from '../utils/apiClient';

// Mock the apiClient
jest.mock('../utils/apiClient', () => ({
  get: jest.fn(),
  post: jest.fn(),
  patch: jest.fn(),
  delete: jest.fn()
}));

// Mock console.error to prevent test logs
console.error = jest.fn();

describe('teamsService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('getTeam', () => {
    it('should fetch a team by id successfully', async () => {
      const mockTeam = { id: 'team-123', name: 'Engineering' };
      apiClient.get.mockResolvedValueOnce({ data: mockTeam });

      const result = await teamsService.getTeam('team-123');

      expect(apiClient.get).toHaveBeenCalledWith('/groups/team-123');
      expect(result).toEqual(mockTeam);
    });

    it('should throw error when fetch fails', async () => {
      const error = new Error('API Error');
      apiClient.get.mockRejectedValueOnce(error);

      await expect(teamsService.getTeam('team-123')).rejects.toThrow('API Error');
      expect(console.error).toHaveBeenCalledWith('Error fetching team:', error);
    });
  });

  describe('getTeams', () => {
    it('should fetch all teams successfully', async () => {
      const mockTeams = [
        { id: 'team-1', name: 'Engineering' },
        { id: 'team-2', name: 'Product' }
      ];
      apiClient.get.mockResolvedValueOnce({ data: mockTeams });

      const result = await teamsService.getTeams();

      expect(apiClient.get).toHaveBeenCalledWith('/groups');
      expect(result).toEqual(mockTeams);
    });

    it('should throw error when fetch fails', async () => {
      const error = new Error('API Error');
      apiClient.get.mockRejectedValueOnce(error);

      await expect(teamsService.getTeams()).rejects.toThrow('API Error');
      expect(console.error).toHaveBeenCalledWith('Error fetching teams:', error);
    });
  });

  describe('createTeam', () => {
    it('should create a team successfully', async () => {
      const teamData = { name: 'New Team', description: 'A team for testing' };
      const mockResponse = { 
        data: { id: 'new-team-id', ...teamData }
      };
      apiClient.post.mockResolvedValueOnce(mockResponse);

      const result = await teamsService.createTeam(teamData);

      expect(apiClient.post).toHaveBeenCalledWith('/groups', teamData);
      expect(result).toEqual(mockResponse.data);
    });

    it('should throw error when creation fails', async () => {
      const teamData = { name: 'New Team' };
      const error = new Error('API Error');
      apiClient.post.mockRejectedValueOnce(error);

      await expect(teamsService.createTeam(teamData)).rejects.toThrow('API Error');
      expect(console.error).toHaveBeenCalledWith('Error creating team:', error);
    });
  });

  describe('updateTeam', () => {
    it('should update a team successfully', async () => {
      const teamId = 'team-123';
      const updateData = { name: 'Updated Team Name' };
      const mockResponse = { 
        data: { id: teamId, name: 'Updated Team Name' }
      };
      apiClient.patch.mockResolvedValueOnce(mockResponse);

      const result = await teamsService.updateTeam(teamId, updateData);

      expect(apiClient.patch).toHaveBeenCalledWith('/groups/team-123', updateData);
      expect(result).toEqual(mockResponse.data);
    });

    it('should throw error when update fails', async () => {
      const teamId = 'team-123';
      const updateData = { name: 'Updated Team Name' };
      const error = new Error('API Error');
      apiClient.patch.mockRejectedValueOnce(error);

      await expect(teamsService.updateTeam(teamId, updateData)).rejects.toThrow('API Error');
      expect(console.error).toHaveBeenCalledWith('Error updating team:', error);
    });
  });

  describe('deleteTeam', () => {
    it('should delete a team successfully', async () => {
      const teamId = 'team-123';
      apiClient.delete.mockResolvedValueOnce({});

      await teamsService.deleteTeam(teamId);

      expect(apiClient.delete).toHaveBeenCalledWith('/groups/team-123');
    });

    it('should throw error when deletion fails', async () => {
      const teamId = 'team-123';
      const error = new Error('API Error');
      apiClient.delete.mockRejectedValueOnce(error);

      await expect(teamsService.deleteTeam(teamId)).rejects.toThrow('API Error');
      expect(console.error).toHaveBeenCalledWith('Error deleting team:', error);
    });
  });
});