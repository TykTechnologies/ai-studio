import { handleApiError } from './errorHandler';

describe('errorHandler', () => {
  describe('handleApiError', () => {
    test('should return error with response data message when available', () => {
      const error = {
        response: {
          data: {
            message: 'API error message'
          }
        }
      };
      
      const result = handleApiError(error);
      
      expect(result).toBeInstanceOf(Error);
      expect(result.message).toBe('API error message');
    });

    test('should return error with error message when response data message is not available', () => {
      const error = {
        message: 'General error message'
      };
      
      const result = handleApiError(error);
      
      expect(result).toBeInstanceOf(Error);
      expect(result.message).toBe('General error message');
    });

    test('should return generic error when no message is available', () => {
      const error = {};
      
      const result = handleApiError(error);
      
      expect(result).toBeInstanceOf(Error);
      expect(result.message).toBe('Unknown error occurred');
    });
  });
});