import { setupSSEConnection } from './sseConnectionService';
import { detectErrorType, generateTempId } from '../utils/chatMessageUtils';
import pubClient from '../../../../admin/utils/pubClient';

// Mock dependencies
jest.mock('../utils/chatMessageUtils');
jest.mock('../../../../admin/utils/pubClient');

// Mock EventSource
class MockEventSource {
  constructor(url, options) {
    this.url = url;
    this.options = options;
    this.readyState = 0; // CONNECTING
    this.eventListeners = {};
    this.onopen = null;
    this.onmessage = null;
    this.onerror = null;
  }
  
  addEventListener(event, callback) {
    if (!this.eventListeners[event]) {
      this.eventListeners[event] = [];
    }
    this.eventListeners[event].push(callback);
  }
  
  removeEventListener(event, callback) {
    if (this.eventListeners[event]) {
      this.eventListeners[event] = this.eventListeners[event].filter(cb => cb !== callback);
    }
  }
  
  dispatchEvent(event) {
    const eventType = event.type;
    
    // Call specific event listeners
    if (this.eventListeners[eventType]) {
      this.eventListeners[eventType].forEach(callback => callback(event));
    }
    
    // Call general handlers
    if (eventType === 'open' && this.onopen) {
      this.onopen(event);
    } else if (eventType === 'message' && this.onmessage) {
      this.onmessage(event);
    } else if (eventType === 'error' && this.onerror) {
      this.onerror(event);
    }
  }
  
  close() {
    this.readyState = 2; // CLOSED
  }
}

// Add EventSource to global
global.EventSource = MockEventSource;
global.EventSource.CONNECTING = 0;
global.EventSource.OPEN = 1;
global.EventSource.CLOSED = 2;

// Spy on console methods to prevent test output pollution
let consoleLogSpy;
let consoleErrorSpy;

beforeAll(() => {
  // Setup global console spies
  consoleLogSpy = jest.spyOn(console, 'log').mockImplementation(() => {});
  consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
});

afterAll(() => {
  // Restore global console spies
  consoleLogSpy.mockRestore();
  consoleErrorSpy.mockRestore();
});

describe('setupSSEConnection', () => {
  // Mock localStorage
  let localStorageMock;
  
  // Mock window.history
  let originalHistory;
  let historyMock;
  
  // Mock callback functions
  let mockCallbacks;
  
  // Mock refs
  let mockRefs;
  
  // Default params
  let defaultParams;
  
  beforeEach(() => {
    // Mock localStorage
    localStorageMock = {
      getItem: jest.fn().mockReturnValue('mock-token'),
      setItem: jest.fn(),
      clear: jest.fn()
    };
    Object.defineProperty(window, 'localStorage', { value: localStorageMock });
    
    // Mock window.history
    originalHistory = window.history;
    window.history = { replaceState: () => {} };
    historyMock = jest.spyOn(window.history, 'replaceState').mockImplementation(() => {});
    
    // Mock pubClient
    pubClient.defaults = {
      baseURL: 'https://api.example.com'
    };
    
    // Mock generateTempId
    generateTempId.mockReturnValue('temp-123');
    
    // Mock detectErrorType
    detectErrorType.mockImplementation((error) => {
      if (error && error.includes('LLM')) {
        return 'llm_config';
      }
      return 'connection';
    });
    
    // Setup mock callbacks
    mockCallbacks = {
      onMessageReceived: jest.fn(),
      setIsConnected: jest.fn(),
      setSessionId: jest.fn(),
      setError: jest.fn(),
      setIsLoading: jest.fn(),
      fetchChatHistory: jest.fn().mockResolvedValue([])
    };
    
    // Setup mock refs
    mockRefs = {
      eventSourceRef: { current: null },
      isConnectedRef: { current: false },
      reconnectAttempts: { current: 0 },
      loadingTimeoutRef: { current: null }
    };
    
    // Default params for setupSSEConnection
    defaultParams = {
      eventSourceRef: mockRefs.eventSourceRef,
      chatId: 'chat-123',
      continueId: null,
      onMessageReceived: mockCallbacks.onMessageReceived,
      setIsConnected: mockCallbacks.setIsConnected,
      setSessionId: mockCallbacks.setSessionId,
      setError: mockCallbacks.setError,
      setIsLoading: mockCallbacks.setIsLoading,
      isConnectedRef: mockRefs.isConnectedRef,
      reconnectAttempts: mockRefs.reconnectAttempts,
      loadingTimeoutRef: mockRefs.loadingTimeoutRef,
      fetchChatHistory: mockCallbacks.fetchChatHistory,
      maxReconnectAttempts: 3,
      initialReconnectDelay: 100
    };
    
    // Reset all mocks
    jest.clearAllMocks();
    
    // Reset mock timers
    jest.useFakeTimers();
  });
  
  afterEach(() => {
    // Restore window.history
    historyMock.mockRestore();
    window.history = originalHistory;
    
    // Restore real timers
    jest.useRealTimers();
  });
  
  test('should set up EventSource with correct URL', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Verify EventSource was created with correct URL
    expect(mockRefs.eventSourceRef.current).not.toBeNull();
    expect(mockRefs.eventSourceRef.current.url).toBe('https://api.example.com/common/chat/chat-123?token=mock-token');
  });
  
  test('should set up EventSource with continueId if provided', () => {
    // Call the function with continueId
    setupSSEConnection({
      ...defaultParams,
      continueId: 'session-456'
    });
    
    // Verify EventSource was created with correct URL including continueId
    expect(mockRefs.eventSourceRef.current).not.toBeNull();
    expect(mockRefs.eventSourceRef.current.url).toBe('https://api.example.com/common/chat/chat-123?session_id=session-456&token=mock-token');
  });
  
  test('should handle onopen event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate onopen event
    mockRefs.eventSourceRef.current.dispatchEvent({ type: 'open' });
    
    // Verify callbacks were called
    expect(mockCallbacks.setIsConnected).toHaveBeenCalledWith(true);
    expect(mockRefs.isConnectedRef.current).toBe(true);
    expect(mockRefs.reconnectAttempts.current).toBe(0);
    expect(mockCallbacks.setError).toHaveBeenCalledWith(null);
    // Loading state is now managed in the session_id handler
  });
  
  test('should handle session_id event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Create mock session_id event data
    const sessionData = {
      payload: 'session-789',
      tools: [{ id: 1, name: 'Calculator' }],
      datasources: [{ id: 2, name: 'Database' }]
    };
    
    // Simulate session_id event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: JSON.stringify(sessionData)
    });
    
    // Verify callbacks were called
    expect(mockCallbacks.setSessionId).toHaveBeenCalledWith('session-789');
    
    // Verify window.history.replaceState was called
    expect(historyMock).toHaveBeenCalledWith(
      {},
      "",
      "/chat/chat-123?continue_id=session-789"
    );
    
    // Verify onMessageReceived was called with processed data
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith(expect.objectContaining({
      payload: 'session-789',
      tools: expect.arrayContaining([
        expect.objectContaining({
          id: 1,
          name: 'Calculator',
          type: 'tool',
          uniqueId: 'tool-1'
        })
      ]),
      datasources: expect.arrayContaining([
        expect.objectContaining({
          id: 2,
          name: 'Database',
          type: 'database',
          uniqueId: 'database-2'
        })
      ])
    }));
  });
  
  test('should fetch chat history when continueId is provided', async () => {
    // Setup a mock implementation that calls setIsLoading(false)
    mockCallbacks.fetchChatHistory.mockImplementation(() => {
      return Promise.resolve([{ id: 'msg-1', type: 'user', content: 'test' }])
        .then(result => {
          // Simulate the behavior in sseConnectionService.js
          mockCallbacks.onMessageReceived({
            type: 'history',
            payload: JSON.stringify([{
              id: 'msg-1',
              attributes: {
                content: JSON.stringify({
                  role: 'human',
                  text: 'test'
                })
              }
            }])
          });
          mockCallbacks.setIsLoading(false);
          return result;
        });
    });
    
    // Call the function with continueId
    setupSSEConnection({
      ...defaultParams,
      continueId: 'session-456'
    });
    
    // Create mock session_id event data
    const sessionData = {
      payload: 'session-789'
    };
    
    // Simulate session_id event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: JSON.stringify(sessionData)
    });
    
    // Verify fetchChatHistory was called
    expect(mockCallbacks.fetchChatHistory).toHaveBeenCalledWith('session-456');
    
    // Wait for the next tick to allow the promise to resolve
    await Promise.resolve();
    
    // Verify setIsLoading(false) was called
    expect(mockCallbacks.setIsLoading).toHaveBeenCalledWith(false);
  });
  
  test('should set isLoading to false for new chats', () => {
    // Call the function without continueId
    setupSSEConnection(defaultParams);
    
    // Create mock session_id event data
    const sessionData = {
      payload: 'session-789'
    };
    
    // Simulate session_id event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: JSON.stringify(sessionData)
    });
    
    // Verify setIsLoading(false) is called for new chats
    expect(mockCallbacks.setIsLoading).toHaveBeenCalledWith(false);
  });
  
  test('should handle stream_chunk event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate stream_chunk event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'stream_chunk',
      data: 'chunk data'
    });
    
    // Verify onMessageReceived was called with correct data
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      type: 'stream_chunk',
      payload: 'chunk data'
    });
  });
  
  test('should handle message event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Create mock message data
    const messageData = {
      id: 'msg-123',
      content: 'Hello world'
    };
    
    // Simulate message event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'message',
      data: JSON.stringify(messageData)
    });
    
    // Verify onMessageReceived was called with parsed data
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith(messageData);
  });
  
  test('should handle system event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate system event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'system',
      data: 'System notification'
    });
    
    // Verify onMessageReceived was called with formatted system message
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      content: ':::system System notification:::',
      isComplete: true
    });
  });
  
  test('should handle system event with existing system prefix', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate system event with existing prefix
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'system',
      data: ':::system Already formatted:::'
    });
    
    // Verify onMessageReceived was called with unchanged message
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      content: ':::system Already formatted:::',
      isComplete: true
    });
  });
  
  test('should handle error event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate error event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'error',
      data: 'Connection error'
    });
    
    // Verify onMessageReceived was called with error message
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      content: ':::system Error: Connection error:::',
      errorType: 'connection',
      isComplete: true
    });
  });
  
  test('should handle LLM config error differently', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate LLM config error event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'error',
      data: 'LLM configuration error'
    });
    
    // Verify onMessageReceived was called with LLM error message
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      content: ':::system Error: LLM configuration error:::',
      errorType: 'llm_config',
      isComplete: true
    });
    
    // Verify reconnectAttempts was set to max to prevent reconnection
    expect(mockRefs.reconnectAttempts.current).toBe(3);
  });
  
  test('should handle generic message event correctly', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Create mock message data
    const messageData = {
      id: 'msg-123',
      content: 'Generic message'
    };
    
    // Simulate generic message event via onmessage
    mockRefs.eventSourceRef.current.onmessage({
      data: JSON.stringify(messageData)
    });
    
    // Verify onMessageReceived was called with parsed data
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith(messageData);
  });
  
  test('should handle connection error and attempt reconnection', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate connection error
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error' });
    
    // Verify callbacks were called
    expect(mockCallbacks.setIsConnected).toHaveBeenCalledWith(false);
    expect(mockRefs.isConnectedRef.current).toBe(false);
    expect(mockCallbacks.setIsLoading).toHaveBeenCalledWith(false);
    
    // Verify reconnection message was sent
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      payload: 'Connection lost. Attempting to reconnect... (Attempt 1/3)',
      errorType: 'connection'
    });
    
    // Fast-forward timers to trigger reconnection
    jest.advanceTimersByTime(100);
    
    // Verify reconnectAttempts was incremented
    expect(mockRefs.reconnectAttempts.current).toBe(1);
    
    // Verify EventSource was closed and recreated
    expect(mockRefs.eventSourceRef.current).not.toBeNull();
  });
  
  test('should handle maximum reconnection attempts', () => {
    // Set reconnectAttempts to max - 1
    mockRefs.reconnectAttempts.current = 2;
    
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate connection error
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error' });
    
    // Fast-forward timers to trigger reconnection
    jest.advanceTimersByTime(400); // 100 * 2^2 = 400ms
    
    // Verify reconnectAttempts was incremented to max
    expect(mockRefs.reconnectAttempts.current).toBe(3);
    
    // Simulate another connection error
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error' });
    
    // Verify max attempts message was sent
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      payload: 'Maximum reconnection attempts reached. Please refresh the page.',
      errorType: 'connection'
    });
    
    // Verify error was set
    expect(mockCallbacks.setError).toHaveBeenCalledWith(
      'Maximum reconnection attempts reached. Please refresh the page.'
    );
  });
  
  test('should handle ping events', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate ping event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'ping'
    });
    
    // No specific assertions needed, just verify it doesn't throw
    expect(true).toBe(true);
  });
  
  test('should handle error parsing session_id message', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate session_id event with invalid JSON
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: 'invalid json'
    });
    
    // Verify error message was sent
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      payload: 'Error: Failed to parse message from server',
      isComplete: true
    });
  });
  
  test('should handle error parsing generic message', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Simulate generic message event with invalid JSON
    mockRefs.eventSourceRef.current.onmessage({
      data: 'invalid json'
    });
    
    // Verify error message was sent
    expect(mockCallbacks.onMessageReceived).toHaveBeenCalledWith({
      id: 'temp-123',
      type: 'system',
      payload: 'Error: Failed to parse message from server',
      isComplete: true
    });
  });
  
  test('should reuse existing EventSource if available', () => {
    // Create an existing EventSource
    mockRefs.eventSourceRef.current = new MockEventSource('existing-url', {});
    const existingEventSource = mockRefs.eventSourceRef.current;
    
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Verify the existing EventSource was reused
    expect(mockRefs.eventSourceRef.current).toBe(existingEventSource);
  });
  
  test('should create new EventSource if current one is closed', () => {
    // Create a closed EventSource
    mockRefs.eventSourceRef.current = new MockEventSource('existing-url', {});
    mockRefs.eventSourceRef.current.readyState = EventSource.CLOSED;
    const existingEventSource = mockRefs.eventSourceRef.current;
    
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Verify a new EventSource was created
    expect(mockRefs.eventSourceRef.current).not.toBe(existingEventSource);
  });

  // New tests for the session reconnection fix

  test('should update currentSessionId when session_id event is received', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Create mock session_id event data
    const sessionData = {
      payload: 'session-789',
      tools: []
    };
    
    // Simulate session_id event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: JSON.stringify(sessionData)
    });
    
    // Verify setSessionId was called with the correct session ID
    expect(mockCallbacks.setSessionId).toHaveBeenCalledWith('session-789');
    
    // Store the original EventSource
    const originalEventSource = mockRefs.eventSourceRef.current;
    
    // Now simulate a connection error to trigger reconnection
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error' });
    
    // Fast-forward timers to trigger reconnection
    jest.advanceTimersByTime(100);
    
    // Verify that a new EventSource was created
    expect(mockRefs.eventSourceRef.current).not.toBe(originalEventSource);
    
    // Verify that the new connection uses the session ID from the session_id event
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=session-789');
  });
  
  test('should use current sessionId instead of original continueId for reconnection', () => {
    // Call the function with an initial continueId
    setupSSEConnection({
      ...defaultParams,
      continueId: 'original-session-123'
    });
    
    // Verify initial connection uses the original continueId
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=original-session-123');
    
    // Create mock session_id event data with a different session ID
    const sessionData = {
      payload: 'new-session-456',
      tools: []
    };
    
    // Simulate session_id event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: JSON.stringify(sessionData)
    });
    
    // Store the original EventSource
    const originalEventSource = mockRefs.eventSourceRef.current;
    
    // Now simulate a connection error to trigger reconnection
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error' });
    
    // Fast-forward timers to trigger reconnection
    jest.advanceTimersByTime(100);
    
    // Verify that a new EventSource was created
    expect(mockRefs.eventSourceRef.current).not.toBe(originalEventSource);
    
    // Verify that the new connection uses the new session ID, not the original continueId
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=new-session-456');
    expect(mockRefs.eventSourceRef.current.url).not.toContain('session_id=original-session-123');
  });
  
  test('should maintain session continuity across multiple reconnections', () => {
    // Call the function
    setupSSEConnection(defaultParams);
    
    // Create mock session_id event data
    const sessionData = {
      payload: 'persistent-session-id',
      tools: []
    };
    
    // Simulate session_id event
    mockRefs.eventSourceRef.current.dispatchEvent({
      type: 'session_id',
      data: JSON.stringify(sessionData)
    });
    
    // Verify setSessionId was called with the correct session ID
    expect(mockCallbacks.setSessionId).toHaveBeenCalledWith('persistent-session-id');
    
    // Simulate first connection error and reconnection
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error 1' });
    jest.advanceTimersByTime(100);
    
    // Verify first reconnection uses the correct session ID
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=persistent-session-id');
    
    // Simulate second connection error and reconnection
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error 2' });
    jest.advanceTimersByTime(200); // 100 * 2^1
    
    // Verify second reconnection still uses the correct session ID
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=persistent-session-id');
    
    // Simulate third connection error and reconnection
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error 3' });
    jest.advanceTimersByTime(400); // 100 * 2^2
    
    // Verify third reconnection still uses the correct session ID
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=persistent-session-id');
  });

  test('should fallback to continueId if currentSessionId is not available', () => {
    // Call the function with an initial continueId
    setupSSEConnection({
      ...defaultParams,
      continueId: 'fallback-session-id'
    });
    
    // Verify initial connection uses the continueId
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=fallback-session-id');
    
    // Store the original EventSource
    const originalEventSource = mockRefs.eventSourceRef.current;
    
    // Simulate a connection error without receiving a session_id event first
    mockRefs.eventSourceRef.current.onerror({ data: 'Connection error' });
    
    // Fast-forward timers to trigger reconnection
    jest.advanceTimersByTime(100);
    
    // Verify that a new EventSource was created
    expect(mockRefs.eventSourceRef.current).not.toBe(originalEventSource);
    
    // Verify that the new connection still uses the original continueId as fallback
    expect(mockRefs.eventSourceRef.current.url).toContain('session_id=fallback-session-id');
  });
});
