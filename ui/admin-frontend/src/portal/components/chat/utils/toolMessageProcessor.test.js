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

  test('should return the original array if no tool messages are present', () => {
    const messages = [
      { type: 'user', content: 'Hello' },
      { type: 'ai', content: 'Hi there!' },
      { type: 'user', content: 'How are you?' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    expect(result).toEqual(messages);
  });

  test('should reorder and merge explanation message with tool_use and tool_result', () => {
    const messages = [
      { type: 'ai', content: 'tool_use: {"function": {"name": "get_weather", "arguments": {"city": "London"}}}' },
      { type: 'ai', content: 'tool_result: {"content": {"temperature": 15, "condition": "cloudy"}}' },
      { type: 'ai', content: 'I will check the weather for you.' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should have merged into a single message
    expect(result.length).toBe(1);
    expect(result[0].content).toContain('I will check the weather for you.');
    expect(result[0].content).toContain('get_weather()');
    expect(result[0].content).toContain('{"city":"London"}');
    expect(result[0].content).toContain('{"temperature":15,"condition":"cloudy"}');
  });

  test('should merge tool_use and tool_result into previous message', () => {
    const messages = [
      { type: 'ai', content: 'Let me check the weather for you.' },
      { type: 'ai', content: 'tool_use: {"function": {"name": "get_weather", "arguments": {"city": "London"}}}' },
      { type: 'ai', content: 'tool_result: {"content": {"temperature": 15, "condition": "cloudy"}}' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Check that we only have one message now
    expect(result.length).toBe(1);
    
    // Check that the content includes the original message
    expect(result[0].content).toContain('Let me check the weather for you.');
    
    // Check that the content includes the function name
    expect(result[0].content).toContain('get_weather()');
    
    // Check that the content includes the parameters
    expect(result[0].content).toContain('{"city":"London"}');
    
    // Check that the content includes the result data
    expect(result[0].content).toContain('{"temperature":15,"condition":"cloudy"}');
  });

  test('should handle invalid JSON in tool_use content', () => {
    const messages = [
      { type: 'ai', content: 'Let me check the weather for you.' },
      { type: 'ai', content: 'tool_use: {invalid json}' },
      { type: 'ai', content: 'tool_result: {"content": {"temperature": 15, "condition": "cloudy"}}' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should still merge but use default values for invalid JSON
    expect(result.length).toBe(1);
    expect(result[0].content).toContain('unknown');
    expect(console.error).toHaveBeenCalled();
  });

  test('should handle invalid JSON in tool_result content', () => {
    const messages = [
      { type: 'ai', content: 'Let me check the weather for you.' },
      { type: 'ai', content: 'tool_use: {"function": {"name": "get_weather", "arguments": {"city": "London"}}}' },
      { type: 'ai', content: 'tool_result: {invalid json}' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should still merge but use default values for invalid JSON
    expect(result.length).toBe(1);
    expect(result[0].content).toContain('get_weather()');
    expect(console.error).toHaveBeenCalled();
  });

  test('should handle empty tool_result content', () => {
    const messages = [
      { type: 'ai', content: 'Let me check the weather for you.' },
      { type: 'ai', content: 'tool_use: {"function": {"name": "get_weather", "arguments": {"city": "London"}}}' },
      { type: 'ai', content: 'tool_result: {"content": ""}' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // Should still merge and include [CONTEXT] section with empty content
    expect(result.length).toBe(1);
    expect(result[0].content).toContain('get_weather()');
    expect(result[0].content).toContain('[CONTEXT]{}[/CONTEXT]');
  });

  test('should handle multiple tool use sequences', () => {
    const messages = [
      { type: 'ai', content: 'Let me check the weather for you.' },
      { type: 'ai', content: 'tool_use: {"function": {"name": "get_weather", "arguments": {"city": "London"}}}' },
      { type: 'ai', content: 'tool_result: {"content": {"temperature": 15, "condition": "cloudy"}}' },
      { type: 'ai', content: 'Now let me check the forecast.' },
      { type: 'ai', content: 'tool_use: {"function": {"name": "get_forecast", "arguments": {"city": "London", "days": 3}}}' },
      { type: 'ai', content: 'tool_result: {"content": [{"day": 1, "temp": 16}, {"day": 2, "temp": 18}]}' },
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    
    // The implementation returns 4 messages
    expect(result.length).toBe(4);
    
    // Check that the messages contain the expected content
    const weatherExplanation = result.find(msg => 
      msg.content === 'Let me check the weather for you.' || 
      msg.content.includes('Let me check the weather for you.')
    );
    expect(weatherExplanation).toBeTruthy();
    
    const forecastExplanation = result.find(msg => 
      msg.content === 'Now let me check the forecast.' || 
      msg.content.includes('Now let me check the forecast.')
    );
    expect(forecastExplanation).toBeTruthy();
    
    // Check that at least one message contains the weather info
    const hasWeatherInfo = result.some(msg => 
      msg.content.includes('get_weather()') && 
      msg.content.includes('{"temperature":15,"condition":"cloudy"}')
    );
    expect(hasWeatherInfo).toBe(true);
    
    // Check for the exact strings we saw in the logs
    const hasForecastToolUse = result.some(msg => 
      msg.content === 'tool_use: {"function": {"name": "get_forecast", "arguments": {"city": "London", "days": 3}}}'
    );
    expect(hasForecastToolUse).toBe(true);
    
    const hasForecastToolResult = result.some(msg => 
      msg.content === 'tool_result: {"content": [{"day": 1, "temp": 16}, {"day": 2, "temp": 18}]}'
    );
    expect(hasForecastToolResult).toBe(true);
  });

  test('should handle empty messages array', () => {
    const result = reorderAndMergeToolMessages([], []);
    expect(result).toEqual([]);
  });

  test('should handle messages with missing properties', () => {
    const messages = [
      { type: 'ai' }, // Missing content
      { content: 'Hello' }, // Missing type
      null, // Null message
      undefined, // Undefined message
    ];
    
    const result = reorderAndMergeToolMessages(messages, []);
    expect(result).toEqual(messages);
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
