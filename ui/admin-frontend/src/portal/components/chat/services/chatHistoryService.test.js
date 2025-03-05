import { fetchChatHistory, formatHistoryForServer, sendChatMessage } from './chatHistoryService';
import pubClient from '../../../../admin/utils/pubClient';
import { parseServerMessage, generateTempId } from '../utils/chatMessageUtils';
import { reorderAndMergeToolMessages } from '../utils/toolMessageProcessor';

// Mock dependencies
jest.mock('../../../../admin/utils/pubClient');
jest.mock('../utils/chatMessageUtils');
jest.mock('../utils/toolMessageProcessor');

describe('fetchChatHistory', () => {
  beforeEach(() => {
    // Setup mocks
    pubClient.get.mockReset();
    parseServerMessage.mockReset();
    reorderAndMergeToolMessages.mockReset();
    generateTempId.mockReset();
    
    // Mock localStorage.getItem
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation((key) => {
      if (key === 'userEntitlements') {
        return JSON.stringify({
          data: {
            tool_catalogues: [{
              attributes: {
                tools: [{ id: 1, name: 'Calculator' }]
              }
            }]
          }
        });
      }
      return null;
    });
    
    // Default implementation for parseServerMessage
    parseServerMessage.mockImplementation(msg => ({
      id: msg.id,
      type: 'ai',
      content: 'Mocked content',
      isComplete: true
    }));
    
    // Default implementation for reorderAndMergeToolMessages
    reorderAndMergeToolMessages.mockImplementation(messages => messages);
    
    // Default implementation for generateTempId
    generateTempId.mockReturnValue('temp-123');
  });
  
  afterEach(() => {
    // Restore localStorage mock
    jest.restoreAllMocks();
  });
  
  test('should fetch and process chat history successfully', async () => {
    // Mock API response
    const mockMessages = [
      { id: '1', attributes: { content: '{"role":"ai","text":"Hello"}' } },
      { id: '2', attributes: { content: '{"role":"human","text":"Hi"}' } }
    ];
    pubClient.get.mockResolvedValue({ data: mockMessages });
    
    // Mock parseServerMessage to return different message types
    parseServerMessage
      .mockReturnValueOnce({
        id: '1',
        type: 'ai',
        content: 'Hello',
        isComplete: true
      })
      .mockReturnValueOnce({
        id: '2',
        type: 'user',
        content: 'Hi',
        isComplete: true
      });
    
    // Mock reorderAndMergeToolMessages
    const mockReorderedMessages = [
      { id: '1', type: 'ai', content: 'Hello', isComplete: true },
      { id: '2', type: 'user', content: 'Hi', isComplete: true }
    ];
    reorderAndMergeToolMessages.mockReturnValue(mockReorderedMessages);
    
    // Call the function
    const result = await fetchChatHistory('session-123');
    
    // Verify API was called correctly
    expect(pubClient.get).toHaveBeenCalledWith('/common/sessions/session-123/messages?limit=100');
    
    // Verify parseServerMessage was called the correct number of times
    expect(parseServerMessage).toHaveBeenCalledTimes(2);
    
    // Verify localStorage was accessed
    expect(Storage.prototype.getItem).toHaveBeenCalledWith('userEntitlements');
    
    // Verify reorderAndMergeToolMessages was called
    expect(reorderAndMergeToolMessages).toHaveBeenCalledWith(
      [
        { id: '1', type: 'ai', content: 'Hello', isComplete: true },
        { id: '2', type: 'user', content: 'Hi', isComplete: true }
      ],
      [{ id: 1, name: 'Calculator' }]
    );
    
    // Verify result
    expect(result).toEqual(mockReorderedMessages);
  });
  
  test('should handle empty response data', async () => {
    // Mock API response with empty data
    pubClient.get.mockResolvedValue({ data: [] });
    
    // Call the function
    const result = await fetchChatHistory('session-123');
    
    // Verify result is empty array
    expect(result).toEqual([]);
    
    // Verify parseServerMessage was not called
    expect(parseServerMessage).not.toHaveBeenCalled();
  });
  
  test('should handle null response data', async () => {
    // Mock API response with null data
    pubClient.get.mockResolvedValue({ data: null });
    
    // Call the function
    const result = await fetchChatHistory('session-123');
    
    // Verify result is empty array
    expect(result).toEqual([]);
    
    // Verify parseServerMessage was not called
    expect(parseServerMessage).not.toHaveBeenCalled();
  });
  
  test('should filter out null messages from parseServerMessage', async () => {
    // Mock API response
    const mockMessages = [
      { id: '1', attributes: { content: '{"role":"ai","text":"Hello"}' } },
      { id: '2', attributes: { content: '{"role":"human","text":"Hi"}' } },
      { id: '3', attributes: { content: '{"role":"unknown","text":"Invalid"}' } }
    ];
    pubClient.get.mockResolvedValue({ data: mockMessages });
    
    // Override the localStorage mock for this test
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      return JSON.stringify({
        data: { tool_catalogues: [] }
      });
    });
    
    // Mock parseServerMessage to return null for the third message
    parseServerMessage
      .mockReturnValueOnce({
        id: '1',
        type: 'ai',
        content: 'Hello',
        isComplete: true
      })
      .mockReturnValueOnce({
        id: '2',
        type: 'user',
        content: 'Hi',
        isComplete: true
      })
      .mockReturnValueOnce(null);
    
    // Call the function
    const result = await fetchChatHistory('session-123');
    
    // Verify parseServerMessage was called for each message
    expect(parseServerMessage).toHaveBeenCalledTimes(3);
    
    // Verify result only includes non-null messages
    expect(result).toHaveLength(2);
    expect(result[0].id).toBe('1');
    expect(result[1].id).toBe('2');
  });
  
  test('should handle API error', async () => {
    // Mock API error
    const mockError = new Error('API error');
    pubClient.get.mockRejectedValue(mockError);
    
    // Call the function
    const result = await fetchChatHistory('session-123');
    
    // Verify error message is returned
    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({
      id: 'temp-123',
      type: 'system',
      content: 'Error: Failed to load chat history',
      isComplete: true
    });
    
    // Verify generateTempId was called
    expect(generateTempId).toHaveBeenCalled();
  });
  
  test('should handle missing userEntitlements in localStorage', async () => {
    // Mock API response
    const mockMessages = [
      { id: '1', attributes: { content: '{"role":"ai","text":"Hello"}' } }
    ];
    pubClient.get.mockResolvedValue({ data: mockMessages });
    
    // Override the localStorage mock for this test
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => null);
    
    // Mock parseServerMessage
    parseServerMessage.mockReturnValue({
      id: '1',
      type: 'ai',
      content: 'Hello',
      isComplete: true
    });
    
    // Call the function
    await fetchChatHistory('session-123');
    
    // Verify reorderAndMergeToolMessages was called with empty tools array
    expect(reorderAndMergeToolMessages).toHaveBeenCalledWith(
      [{ id: '1', type: 'ai', content: 'Hello', isComplete: true }],
      []
    );
  });
});

describe('formatHistoryForServer', () => {
  test('should format user messages correctly', () => {
    const messages = [
      { id: '1', type: 'user', content: 'Hello', isComplete: true }
    ];
    
    const result = formatHistoryForServer(messages);
    const parsed = JSON.parse(result);
    
    expect(parsed).toHaveLength(1);
    expect(parsed[0].id).toBe('1');
    expect(parsed[0].attributes.content).toBe(JSON.stringify({
      role: 'human',
      text: 'Hello'
    }));
  });
  
  test('should format AI messages correctly', () => {
    const messages = [
      { id: '2', type: 'ai', content: 'Hi there', isComplete: true }
    ];
    
    const result = formatHistoryForServer(messages);
    const parsed = JSON.parse(result);
    
    expect(parsed).toHaveLength(1);
    expect(parsed[0].id).toBe('2');
    expect(parsed[0].attributes.content).toBe(JSON.stringify({
      role: 'ai',
      text: 'Hi there'
    }));
  });
  
  test('should format system messages correctly', () => {
    const messages = [
      { id: '3', type: 'system', content: 'System notification', isComplete: true }
    ];
    
    const result = formatHistoryForServer(messages);
    const parsed = JSON.parse(result);
    
    expect(parsed).toHaveLength(1);
    expect(parsed[0].id).toBe('3');
    expect(parsed[0].attributes.content).toBe(JSON.stringify({
      role: 'system',
      text: 'System notification'
    }));
  });
  
  test('should format multiple message types correctly', () => {
    const messages = [
      { id: '1', type: 'user', content: 'Hello', isComplete: true },
      { id: '2', type: 'ai', content: 'Hi there', isComplete: true },
      { id: '3', type: 'system', content: 'System notification', isComplete: true }
    ];
    
    const result = formatHistoryForServer(messages);
    const parsed = JSON.parse(result);
    
    expect(parsed).toHaveLength(3);
    
    expect(parsed[0].attributes.content).toBe(JSON.stringify({
      role: 'human',
      text: 'Hello'
    }));
    
    expect(parsed[1].attributes.content).toBe(JSON.stringify({
      role: 'ai',
      text: 'Hi there'
    }));
    
    expect(parsed[2].attributes.content).toBe(JSON.stringify({
      role: 'system',
      text: 'System notification'
    }));
  });
  
  test('should handle empty messages array', () => {
    const result = formatHistoryForServer([]);
    expect(result).toBe('[]');
  });
});

describe('sendChatMessage', () => {
  beforeEach(() => {
    pubClient.post.mockReset();
  });
  
  test('should send message successfully', async () => {
    // Mock successful API call
    pubClient.post.mockResolvedValue({});
    
    // Call the function
    const result = await sendChatMessage('chat-123', 'session-456', { content: 'Hello' });
    
    // Verify API was called correctly
    expect(pubClient.post).toHaveBeenCalledWith(
      '/common/chat/chat-123/messages?session_id=session-456',
      { content: 'Hello' }
    );
    
    // Verify result
    expect(result).toBe(true);
  });
  
  test('should handle API error', async () => {
    // Mock API error
    pubClient.post.mockRejectedValue(new Error('API error'));
    
    // Call the function
    const result = await sendChatMessage('chat-123', 'session-456', { content: 'Hello' });
    
    // Verify result
    expect(result).toBe(false);
  });
  
  test('should handle missing sessionId', async () => {
    // Call the function with null sessionId
    const result = await sendChatMessage('chat-123', null, { content: 'Hello' });
    
    // Verify API was not called
    expect(pubClient.post).not.toHaveBeenCalled();
    
    // Verify result
    expect(result).toBe(false);
  });
  
  test('should handle empty sessionId', async () => {
    // Call the function with empty sessionId
    const result = await sendChatMessage('chat-123', '', { content: 'Hello' });
    
    // Verify API was not called
    expect(pubClient.post).not.toHaveBeenCalled();
    
    // Verify result
    expect(result).toBe(false);
  });
});
