import axios from 'axios';
import * as urlUtils from './urlUtils';

// Mock axios
jest.mock('axios');

describe('urlUtils', () => {
  let mockConsoleError;

  beforeEach(() => {
    // Mock console.error
    mockConsoleError = jest.spyOn(console, 'error').mockImplementation();
    
    // Clear axios mocks before each test
    axios.get.mockClear();
  });

  afterEach(() => {
    // Restore console.error
    mockConsoleError.mockRestore();
    
    // Clear all mocks
    jest.clearAllMocks();
  });

  describe('fetchCSRFToken', () => {
    test('should fetch CSRF token successfully', async () => {
      // Setup
      const mockToken = 'csrf-token-123';
      axios.get.mockResolvedValue({
        headers: {
          'x-csrf-token': mockToken
        }
      });

      // Execute
      const result = await urlUtils.fetchCSRFToken();

      // Verify
      expect(axios.get).toHaveBeenCalledTimes(1);
      expect(axios.get).toHaveBeenCalledWith(expect.any(String), {
        withCredentials: true
      });
      expect(result).toBe(mockToken);
      expect(mockConsoleError).not.toHaveBeenCalled();
    });

    test('should return null when token is not in headers', async () => {
      // Setup
      axios.get.mockResolvedValue({
        headers: {}
      });

      // Execute
      const result = await urlUtils.fetchCSRFToken();

      // Verify
      expect(axios.get).toHaveBeenCalledTimes(1);
      expect(axios.get).toHaveBeenCalledWith(expect.any(String), {
        withCredentials: true
      });
      expect(result).toBeUndefined();
      expect(mockConsoleError).not.toHaveBeenCalled();
    });

    test('should handle network error correctly', async () => {
      // Setup
      const networkError = new Error('Network error');
      axios.get.mockRejectedValue(networkError);

      // Execute
      const result = await urlUtils.fetchCSRFToken();

      // Verify
      expect(axios.get).toHaveBeenCalledTimes(1);
      expect(axios.get).toHaveBeenCalledWith(expect.any(String), {
        withCredentials: true
      });
      expect(result).toBeNull();
      expect(mockConsoleError).toHaveBeenCalledWith('Error fetching CSRF token:', networkError);
    });

    test('should handle server error correctly', async () => {
      // Setup
      const serverError = {
        response: {
          status: 500,
          statusText: 'Internal Server Error'
        }
      };
      axios.get.mockRejectedValue(serverError);

      // Execute
      const result = await urlUtils.fetchCSRFToken();

      // Verify
      expect(axios.get).toHaveBeenCalledTimes(1);
      expect(axios.get).toHaveBeenCalledWith(expect.any(String), {
        withCredentials: true
      });
      expect(result).toBeNull();
      expect(mockConsoleError).toHaveBeenCalledWith('Error fetching CSRF token:', serverError);
    });
  });
});