import { parseServerMessage, detectErrorType, generateTempId, createSystemMessage } from './chatMessageUtils';

describe('parseServerMessage', () => {
// Spy on console.log and console.error to prevent test output pollution
  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.log.mockRestore();
    console.error.mockRestore();
  });
  
  test('should parse a human message correctly', () => {
    const serverMessage = {
      id: '123',
      attributes: {
        content: JSON.stringify({
          role: 'human',
          text: 'Hello world'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '123',
      type: 'user',
      content: 'Hello world',
      isComplete: true
    });
  });
  
  test('should parse an AI message correctly', () => {
    const serverMessage = {
      id: '456',
      attributes: {
        content: JSON.stringify({
          role: 'ai',
          text: 'I am an AI assistant'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '456',
      type: 'ai',
      content: 'I am an AI assistant',
      isComplete: true
    });
  });
  
  test('should parse a system message correctly', () => {
    const serverMessage = {
      id: '789',
      attributes: {
        content: JSON.stringify({
          role: 'system',
          text: 'This is a system message'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '789',
      type: 'system',
      content: ':::system This is a system message:::',
      isComplete: true
    });
  });
  
  test('should parse a tool message as AI message', () => {
    const serverMessage = {
      id: '101',
      attributes: {
        content: JSON.stringify({
          role: 'tool',
          text: 'Tool result'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '101',
      type: 'ai',
      content: 'Tool result',
      isComplete: true
    });
  });
  
  test('should handle messages with context', () => {
    const serverMessage = {
      id: '202',
      attributes: {
        content: JSON.stringify({
          role: 'ai',
          text: 'Here is the information',
          context: '{"data": "Some context data"}'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '202',
      type: 'ai',
      content: '[CONTEXT]{"data": "Some context data"}[/CONTEXT]Here is the information',
      isComplete: true
    });
  });
  
  test('should handle messages with direct content property', () => {
    const serverMessage = {
      id: '303',
      content: JSON.stringify({
        role: 'human',
        text: 'Direct content'
      })
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '303',
      type: 'user',
      content: 'Direct content',
      isComplete: true
    });
  });
  
  test('should handle system messages that already have :::system prefix', () => {
    const serverMessage = {
      id: '404',
      attributes: {
        content: JSON.stringify({
          role: 'system',
          text: ':::system Already prefixed:::'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '404',
      type: 'system',
      content: ':::system Already prefixed:::',
      isComplete: true
    });
  });
  
  test('should return null for messages with missing ID', () => {
    const serverMessage = {
      attributes: {
        content: JSON.stringify({
          role: 'ai',
          text: 'No ID here'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toBeNull();
  });
  
  test('should handle invalid JSON by treating as AI message', () => {
    const serverMessage = {
      id: '505',
      attributes: {
        content: 'Not valid JSON'
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toEqual({
      id: '505',
      type: 'ai',
      content: 'Not valid JSON',
      isComplete: true
    });
  });
  
  test('should return null for unknown role', () => {
    const serverMessage = {
      id: '606',
      attributes: {
        content: JSON.stringify({
          role: 'unknown_role',
          text: 'Unknown role message'
        })
      }
    };
    
    const result = parseServerMessage(serverMessage);
    
    expect(result).toBeNull();
  });
});

describe('detectErrorType', () => {
  test('should detect LLM configuration errors', () => {
    expect(detectErrorType('Failed to create message due to LLM error')).toBe('llm_config');
    expect(detectErrorType('OpenAI API key is invalid')).toBe('llm_config');
    expect(detectErrorType('Model not found')).toBe('llm_config');
    expect(detectErrorType('Anthropic API returned an error')).toBe('llm_config');
  });
  
  test('should detect connection errors', () => {
    expect(detectErrorType('Connection timeout')).toBe('connection');
    expect(detectErrorType('Network error occurred')).toBe('connection');
    expect(detectErrorType('Timeout occurred')).toBe('connection');
  });
  
  test('should return "other" for unrecognized errors', () => {
    expect(detectErrorType('Some random error')).toBe('other');
    expect(detectErrorType('Unknown issue occurred')).toBe('other');
  });
  
  test('should return "connection" for null or undefined errors', () => {
    expect(detectErrorType(null)).toBe('connection');
    expect(detectErrorType(undefined)).toBe('connection');
  });
  
  test('should handle Error objects', () => {
    expect(detectErrorType(new Error('Connection failed'))).toBe('connection');
    expect(detectErrorType(new Error('LLM configuration issue'))).toBe('llm_config');
  });
});

describe('generateTempId', () => {
  test('should generate a string starting with temp_', () => {
    const id = generateTempId();
    expect(typeof id).toBe('string');
    expect(id.startsWith('temp_')).toBe(true);
  });
  
  test('should generate unique IDs on multiple calls', () => {
    const id1 = generateTempId();
    const id2 = generateTempId();
    const id3 = generateTempId();
    
    expect(id1).not.toEqual(id2);
    expect(id1).not.toEqual(id3);
    expect(id2).not.toEqual(id3);
  });
  
  test('should generate numeric part after temp_ prefix', () => {
    const id = generateTempId();
    const numericPart = id.split('_')[1];
    
    expect(Number.isNaN(Number(numericPart))).toBe(false);
  });
});

describe('createSystemMessage', () => {
  test('should create a system message with proper format', () => {
    const result = createSystemMessage('System notification');
    
    expect(result).toEqual({
      id: expect.stringMatching(/^temp_\d+$/),
      type: 'system',
      content: ':::system System notification:::',
      isComplete: true
    });
  });
  
  test('should not add :::system prefix if already present', () => {
    const result = createSystemMessage(':::system Already prefixed:::');
    
    expect(result).toEqual({
      id: expect.stringMatching(/^temp_\d+$/),
      type: 'system',
      content: ':::system Already prefixed:::',
      isComplete: true
    });
  });
  
  test('should add errorType if provided', () => {
    const result = createSystemMessage('Error occurred', 'connection');
    
    expect(result).toEqual({
      id: expect.stringMatching(/^temp_\d+$/),
      type: 'system',
      content: ':::system Error occurred:::',
      errorType: 'connection',
      isComplete: true
    });
  });
  
  test('should not add errorType if null', () => {
    const result = createSystemMessage('No error', null);
    
    expect(result).toEqual({
      id: expect.stringMatching(/^temp_\d+$/),
      type: 'system',
      content: ':::system No error:::',
      isComplete: true
    });
    
    expect(result.errorType).toBeUndefined();
  });
});
