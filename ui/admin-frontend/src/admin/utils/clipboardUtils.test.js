import { copyToClipboard } from './clipboardUtils';

describe('clipboardUtils', () => {
  let originalClipboard;
  let mockWriteText;
  let mockConsoleLog;
  let mockConsoleError;

  beforeEach(() => {
    // Save original clipboard API
    originalClipboard = { ...navigator.clipboard };
    
    // Mock clipboard API
    mockWriteText = jest.fn();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: mockWriteText },
      configurable: true,
    });
    
    // Mock console methods
    mockConsoleLog = jest.spyOn(console, 'log').mockImplementation();
    mockConsoleError = jest.spyOn(console, 'error').mockImplementation();
  });

  afterEach(() => {
    // Restore original clipboard API
    Object.defineProperty(navigator, 'clipboard', {
      value: originalClipboard,
      configurable: true,
    });
    
    // Restore console methods
    mockConsoleLog.mockRestore();
    mockConsoleError.mockRestore();
    
    // Clear all mocks
    jest.clearAllMocks();
  });

  describe('copyToClipboard', () => {
    test('should copy text to clipboard successfully', async () => {
      // Setup
      mockWriteText.mockResolvedValue(undefined);
      const onSuccess = jest.fn();
      const onError = jest.fn();
      const text = 'Test text';
      
      // Execute
      const result = await copyToClipboard(text, null, onSuccess, onError);
      
      // Verify
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleLog).toHaveBeenCalledWith('Text copied to clipboard');
      expect(onSuccess).toHaveBeenCalled();
      expect(onError).not.toHaveBeenCalled();
      expect(result).toBe(true);
    });

    test('should copy text with field name to clipboard successfully', async () => {
      // Setup
      mockWriteText.mockResolvedValue(undefined);
      const onSuccess = jest.fn();
      const onError = jest.fn();
      const text = 'Test text';
      const fieldName = 'API Key';
      
      // Execute
      const result = await copyToClipboard(text, fieldName, onSuccess, onError);
      
      // Verify
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleLog).toHaveBeenCalledWith('Text (API Key) copied to clipboard');
      expect(onSuccess).toHaveBeenCalledWith(fieldName);
      expect(onError).not.toHaveBeenCalled();
      expect(result).toBe(true);
    });

    test('should handle clipboard error correctly', async () => {
      // Setup
      const clipboardError = new Error('Clipboard error');
      mockWriteText.mockRejectedValue(clipboardError);
      const onSuccess = jest.fn();
      const onError = jest.fn();
      const text = 'Test text';
      
      // Execute
      const result = await copyToClipboard(text, null, onSuccess, onError);
      
      // Verify
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleError).toHaveBeenCalledWith('Failed to copy text: ', clipboardError);
      expect(onSuccess).not.toHaveBeenCalled();
      expect(onError).toHaveBeenCalledWith(null, clipboardError);
      expect(result).toBe(false);
    });

    test('should handle clipboard error with field name correctly', async () => {
      // Setup
      const clipboardError = new Error('Clipboard error');
      mockWriteText.mockRejectedValue(clipboardError);
      const onSuccess = jest.fn();
      const onError = jest.fn();
      const text = 'Test text';
      const fieldName = 'API Key';
      
      // Execute
      const result = await copyToClipboard(text, fieldName, onSuccess, onError);
      
      // Verify
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleError).toHaveBeenCalledWith('Failed to copy text (API Key): ', clipboardError);
      expect(onSuccess).not.toHaveBeenCalled();
      expect(onError).toHaveBeenCalledWith(fieldName, clipboardError);
      expect(result).toBe(false);
    });

    test('should work without callback functions', async () => {
      // Setup
      mockWriteText.mockResolvedValue(undefined);
      const text = 'Test text';
      
      // Execute
      const result = await copyToClipboard(text);
      
      // Verify
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleLog).toHaveBeenCalledWith('Text copied to clipboard');
      expect(result).toBe(true);
    });

    test('should handle error without callback functions', async () => {
      // Setup
      const clipboardError = new Error('Clipboard error');
      mockWriteText.mockRejectedValue(clipboardError);
      const text = 'Test text';
      
      // Execute
      const result = await copyToClipboard(text);
      
      // Verify
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleError).toHaveBeenCalledWith('Failed to copy text: ', clipboardError);
      expect(result).toBe(false);
    });
    
    test('should return false and log error with invalid success callback', async () => {
      const text = 'Test text';
      const fieldName = 'API Key';
      const invalidCallback = 'not a function';
      
      mockWriteText.mockResolvedValue(undefined);
      
      const result = await copyToClipboard(text, fieldName, invalidCallback);
      
      expect(result).toBe(false);
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleError).toHaveBeenCalled();
      expect(mockConsoleError.mock.calls[0][0]).toContain('Failed to copy text (API Key)');
    });
    
    test('should throw TypeError with invalid error callback', async () => {
      const text = 'Test text';
      const fieldName = 'API Key';
      const invalidCallback = 'not a function';
      
      const clipboardError = new Error('Clipboard error');
      mockWriteText.mockRejectedValue(clipboardError);
      
      await expect(
        copyToClipboard(text, fieldName, null, invalidCallback)
      ).rejects.toThrow(TypeError);
      
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleError).toHaveBeenCalledWith('Failed to copy text (API Key): ', clipboardError);
    });
    
    test('should handle undefined callbacks gracefully', async () => {
      const text = 'Test text';
      const fieldName = 'API Key';
      
      mockWriteText.mockResolvedValue(undefined);
      
      let result = await copyToClipboard(text, fieldName, undefined);
      
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleLog).toHaveBeenCalledWith('Text (API Key) copied to clipboard');
      expect(result).toBe(true);
      
      jest.clearAllMocks();
      
      const clipboardError = new Error('Clipboard error');
      mockWriteText.mockRejectedValue(clipboardError);
      
      result = await copyToClipboard(text, fieldName, null, undefined);
      
      expect(mockWriteText).toHaveBeenCalledWith(text);
      expect(mockConsoleError).toHaveBeenCalledWith('Failed to copy text (API Key): ', clipboardError);
      expect(result).toBe(false);
    });
  });
});
