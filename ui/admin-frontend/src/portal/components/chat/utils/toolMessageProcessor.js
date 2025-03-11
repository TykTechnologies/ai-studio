export const reorderAndMergeToolMessages = (messages, tools) => {
  console.log("Reordering and merging tool messages");
  console.log("messages");
  console.log(messages);
  
  // Helper function to identify tool calls (only new format)
  const isToolCall = (message) => {
    // New format tool call
    if (message?.type === 'ai' && 
        typeof message.content === 'object' && 
        message.content.parts && 
        Array.isArray(message.content.parts) && 
        message.content.parts[0]?.type === 'tool_call') {
      return true;
    }
    
    // We handle legacy format tool calls separately now
    return false;
  };
  
  // Helper function to identify tool responses
  const isToolResponse = (message) => {
    // New format tool response
    if (message?.type === 'ai' && 
        typeof message.content === 'object' && 
        message.content.parts && 
        Array.isArray(message.content.parts) && 
        message.content.parts[0]?.type === 'tool_response') {
      return true;
    }
    
    // Legacy format tool result
    if (message?.type === 'ai' && 
        typeof message.content === 'string' && 
        (message.content.includes('tool_result') || message.content.includes('/tool_result'))) {
      return true;
    }
    
    // Message with role: 'tool'
    if (message?.type === 'ai' && 
        typeof message.content === 'object' && 
        message.content.role === 'tool') {
      return true;
    }
    
    return false;
  };
  
  // Helper function to normalize content
  const normalizeContent = (message) => {
    if (!message || !message.content) {
      return "";
    }
    
    if (typeof message.content === 'string') {
      return message.content;
    }
    
    if (message.content.text) {
      return message.content.text;
    }
    
    if (message.content.role && message.content.text) {
      return message.content.text;
    }
    
    return JSON.stringify(message.content);
  };
  
  // First pass: normalize all message contents and filter out null/undefined messages
  const normalizedMessages = messages
    .filter(message => message !== null && message !== undefined)
    .map(message => ({
      ...message,
      content: normalizeContent(message)
    }));
  
  // Filter out tool response messages and handle tool calls
  const filteredMessages = [];
  for (const message of normalizedMessages) {
    const isResponse = isToolResponse(message);
    const isNewFormatToolCall = isToolCall(message);
    // More precise detection of legacy format tool calls
    const isLegacyToolCall = message.type === 'ai' && 
                           typeof message.content === 'string' && 
                           message.content.trim().startsWith('tool_use\n{') && 
                           message.content.includes('/tool_use');
    
    if (isResponse || isNewFormatToolCall) {
      continue;
    }
    
    if (isLegacyToolCall) {
      try {
        // Convert legacy tool call to system message
        const toolUseRaw = message.content.replace(/\/?tool_use\s*:?/ig, '').trim();
        let functionName = "unknown";
        let parameters = {};
        
        try {
          const toolUseData = JSON.parse(toolUseRaw);
          functionName = toolUseData?.function?.name || functionName;
          parameters = toolUseData?.function?.arguments || parameters;
        } catch (err) {
          console.error('Error parsing tool_use JSON:', err);
          // If we can't parse the JSON, just keep the original message
          filteredMessages.push(message);
          continue;
        }
        
        // Create system message
        filteredMessages.push({
          id: message.id,
          type: 'ai',
          content: `:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` called:::\n`,
          isComplete: true
        });
      } catch (err) {
        console.error('Error processing legacy tool call:', err);
        // If there's any error, keep the original message
        filteredMessages.push(message);
      }
      continue;
    }
    
    // Keep all other messages
    filteredMessages.push(message);
  }
  
  // Group consecutive AI messages
  const result = [];
  let currentGroup = null;
  
  for (let i = 0; i < filteredMessages.length; i++) {
    const message = filteredMessages[i];
    
    if (message.type === 'user' || message.type === 'system') {
      // If we have a current AI message group, add it to the result
      if (currentGroup) {
        currentGroup.type = 'ai'; // Ensure type is set
        result.push(currentGroup);
        currentGroup = null;
      }
      
      result.push(message);
    } 
    else if (message.type === 'ai') {
      // If we don't have a current group, start one with this message
      if (!currentGroup) {
        currentGroup = { ...message, type: 'ai' }; // Ensure type is set
      } 
      // Otherwise, append this message's content to the current group
      else {
        currentGroup.content += '\n' + message.content;
        currentGroup.type = 'ai'; // Ensure type is set
      }
    }
    else {
      result.push(message);
    }
  }
  
  // Add the last group if there is one
  if (currentGroup) {
    currentGroup.type = 'ai';
    result.push(currentGroup);
  }
  
  console.log("Final result:", result);
  return result;
};

export const processToolsAndDatasources = (data) => {
  if (!data) {
    return data;
  }
  
  if (Array.isArray(data.tools)) {
    data.tools.forEach(tool => {
      const uniqueId = `tool-${tool.id}`;
      tool.type = 'tool';
      tool.uniqueId = uniqueId;
    });
  }
  
  if (Array.isArray(data.datasources)) {
    data.datasources.forEach(ds => {
      const uniqueId = `database-${ds.id}`;
      ds.type = 'database';
      ds.uniqueId = uniqueId;
    });
  }
  
  return data;
};
