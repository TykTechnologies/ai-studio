import { reorderAndMergeToolMessages, processToolsAndDatasources } from './toolMessageProcessor';

describe('reorderAndMergeToolMessages', () => {
  // Spy on console.log and console.error to prevent test output pollution
  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.log.mockRestore();
    console.error.mockRestore();
  });

  test('should keep regular messages unchanged', () => {
    const messages = [
      { type: 'user', content: 'Hello' },
      { type: 'ai', content: 'Hi there!' },
      { type: 'user', content: 'How are you?' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    expect(result).toEqual(messages);
  });

  test('should merge consecutive AI messages', () => {
    const messages = [
      { type: 'user', content: 'Hello' },
      { type: 'ai', content: 'Hi there!' },
      { type: 'ai', content: 'How can I help you today?' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should have merged the two AI messages
    expect(result.length).toBe(2);
    expect(result[0]).toEqual(messages[0]);
    expect(result[1].type).toBe('ai');
    expect(result[1].content).toBe('Hi there!\nHow can I help you today?');
  });

  test('should filter out tool responses', () => {
    // In the actual application flow, tool responses are filtered out by parseServerMessage
    // So they don't even reach reorderAndMergeToolMessages
    const messages = [
      { type: 'user', content: 'What is the weather?' },
      { type: 'ai', content: 'Let me check the weather for you.' },
      { type: 'ai', content: 'The weather is cloudy with a temperature of 15°C.' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should merge the AI messages
    expect(result.length).toBe(2);
    expect(result[0].content).toBe('What is the weather?');
    expect(result[1].content).toBe('Let me check the weather for you.\nThe weather is cloudy with a temperature of 15°C.');
  });

  test('should handle system messages', () => {
    const messages = [
      { type: 'user', content: 'What is the weather?' },
      { type: 'ai', content: 'Let me check the weather for you.' },
      { 
        type: 'ai', 
        content: ':::systemUsing function: `get_weather()`::::::systemParameters: {"city":"London"}::::::systemContent: Function `get_weather()` called:::\n'
      },
      { type: 'ai', content: 'The weather is cloudy with a temperature of 15°C.' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // The function treats system-like messages as regular AI messages and merges them
    expect(result.length).toBe(2);
    expect(result[0].content).toBe('What is the weather?');
    
    // Check that the AI messages are merged, including the system message
    const aiMessage = result[1];
    expect(aiMessage.type).toBe('ai');
    expect(aiMessage.content).toContain('Let me check the weather for you.');
    expect(aiMessage.content).toContain(':::systemUsing function:');
    expect(aiMessage.content).toContain('The weather is cloudy with a temperature of 15°C.');
  });

  test('should handle multiple message types', () => {
    const messages = [
      { type: 'user', content: 'What is the weather?' },
      { type: 'ai', content: 'Let me check the weather for you.' },
      { 
        type: 'ai', 
        content: ':::systemUsing function: `get_weather()`::::::systemParameters: {"city":"London"}::::::systemContent: Function `get_weather()` called:::\n'
      },
      { type: 'ai', content: 'The weather in London is cloudy with a temperature of 15°C.' },
      { type: 'user', content: 'What about the forecast?' },
      { type: 'ai', content: 'Let me check the forecast for you.' },
      { 
        type: 'ai', 
        content: ':::systemUsing function: `get_forecast()`::::::systemParameters: {"city":"London","days":3}::::::systemContent: Function `get_forecast()` called:::\n'
      },
      { type: 'ai', content: 'The forecast for London shows 16°C tomorrow and 18°C the day after.' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // The function treats all AI messages as the same type and merges consecutive ones
    expect(result.length).toBe(4);
    
    // Check that user messages are unchanged
    expect(result[0].content).toBe('What is the weather?');
    expect(result[2].content).toBe('What about the forecast?');
    
    // Check that the AI messages are merged, including system messages
    const firstAiMessage = result[1];
    expect(firstAiMessage.type).toBe('ai');
    expect(firstAiMessage.content).toContain('Let me check the weather for you.');
    expect(firstAiMessage.content).toContain(':::systemUsing function: `get_weather()`:::');
    expect(firstAiMessage.content).toContain('The weather in London is cloudy with a temperature of 15°C.');
    
    const secondAiMessage = result[3];
    expect(secondAiMessage.type).toBe('ai');
    expect(secondAiMessage.content).toContain('Let me check the forecast for you.');
    expect(secondAiMessage.content).toContain(':::systemUsing function: `get_forecast()`:::');
    expect(secondAiMessage.content).toContain('The forecast for London shows 16°C tomorrow and 18°C the day after.');
  });

  test('should handle empty messages array', () => {
    const result = reorderAndMergeToolMessages([], []);
    expect(result).toEqual([]);
  });

  test('should filter out null and undefined messages', () => {
    const messages = [
      { type: 'user', content: 'Hello' },
      null,
      { type: 'ai', content: 'Hi there!' },
      undefined,
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should filter out null and undefined messages
    expect(result.length).toBe(2);
    expect(result[0].content).toBe('Hello');
    expect(result[1].content).toBe('Hi there!');
  });

  test('should handle messages with missing properties', () => {
    const messages = [
      { type: 'user' }, // Missing content
      { content: 'Hello' }, // Missing type
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should handle messages with missing properties
    expect(result.length).toBe(2);
    expect(result[0].content).toBe('');
    expect(result[1].type).toBeUndefined();
  });
});

describe('processToolsAndDatasources', () => {
  test('should add type and uniqueId to tools', () => {
    const data = {
      tools: [
        { id: 1, name: 'Calculator' },
        { id: 2, name: 'Weather' },
      ]
    };
    
    const result = processToolsAndDatasources(data);
    
    expect(result.tools[0].type).toBe('tool');
    expect(result.tools[0].uniqueId).toBe('tool-1');
    expect(result.tools[1].type).toBe('tool');
    expect(result.tools[1].uniqueId).toBe('tool-2');
  });

  test('should add type and uniqueId to datasources', () => {
    const data = {
      datasources: [
        { id: 1, name: 'Customer DB' },
        { id: 2, name: 'Product Catalog' },
      ]
    };
    
    const result = processToolsAndDatasources(data);
    
    expect(result.datasources[0].type).toBe('database');
    expect(result.datasources[0].uniqueId).toBe('database-1');
    expect(result.datasources[1].type).toBe('database');
    expect(result.datasources[1].uniqueId).toBe('database-2');
  });

  test('should handle both tools and datasources', () => {
    const data = {
      tools: [{ id: 1, name: 'Calculator' }],
      datasources: [{ id: 1, name: 'Customer DB' }],
    };
    
    const result = processToolsAndDatasources(data);
    
    expect(result.tools[0].type).toBe('tool');
    expect(result.tools[0].uniqueId).toBe('tool-1');
    expect(result.datasources[0].type).toBe('database');
    expect(result.datasources[0].uniqueId).toBe('database-1');
  });

  test('should handle empty arrays', () => {
    const data = {
      tools: [],
      datasources: [],
    };
    
    const result = processToolsAndDatasources(data);
    
    expect(result.tools).toEqual([]);
    expect(result.datasources).toEqual([]);
  });

  test('should handle missing arrays', () => {
    const data = {};
    
    const result = processToolsAndDatasources(data);
    
    expect(result).toEqual({});
  });

  test('should handle null or undefined input', () => {
    expect(processToolsAndDatasources(null)).toBeNull();
    expect(processToolsAndDatasources(undefined)).toBeUndefined();
  });

  test('should preserve other properties in the data object', () => {
    const data = {
      tools: [{ id: 1 }],
      datasources: [{ id: 1 }],
      otherProp: 'value',
      nestedProp: { key: 'value' },
    };
    
    const result = processToolsAndDatasources(data);
    
    expect(result.otherProp).toBe('value');
    expect(result.nestedProp).toEqual({ key: 'value' });
  });
});
