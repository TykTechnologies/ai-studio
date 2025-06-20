export const parseServerMessage = (msg) => {
  try {    
    const content = msg.attributes?.content || msg.content;
    const messageId = msg.id;
    
    if (!messageId) {
      console.error('Message ID missing from server response:', msg);
      return null;
    }
    
    // Extract and normalize content
    const { parsedContent, messageText } = extractContent(content);
    
    // Skip tool responses
    if (isToolResponse(parsedContent)) {
      return null;
    }
    
    // Handle tool calls
    if (isToolCall(parsedContent)) {
      return createToolCallMessage(messageId, parsedContent);
    }
    
    // Determine message type based on role
    return createMessageFromRole(messageId, parsedContent, messageText);
  } catch (e) {
    console.error('Error parsing server message:', e, msg);
    // If all else fails, treat it as an AI message with direct content
    return {
      id: msg.id,
      type: "ai",
      content: typeof msg.content === 'string' ? msg.content : JSON.stringify(msg.content),
      isComplete: true,
    };
  }
};

// Helper function to extract and normalize content
const extractContent = (content) => {  
  // If content is already an object
  if (typeof content === 'object' && content !== null) {
    return {
      parsedContent: content,
      messageText: content.text || ''
    };
  }
  
  // Check for legacy tool_use format
  if (typeof content === 'string' && content.includes('tool_use')) {
    return {
      parsedContent: content,
      messageText: content
    };
  }
  
  // Try to parse content as JSON
  try {
    const parsedContent = JSON.parse(content);
    
    const messageText = parsedContent.context
      ? `[CONTEXT]${parsedContent.context}[/CONTEXT]${parsedContent.text}`
      : parsedContent.text;
    
    return { parsedContent, messageText };
  } catch (e) {
    return {
      parsedContent: { role: 'ai' },
      messageText: content
    };
  }
};

// Helper function to check if content is a tool call
const isToolCall = (parsedContent) => {
  // Check for new format tool calls
  if (parsedContent.parts && 
      Array.isArray(parsedContent.parts) && 
      parsedContent.parts[0]?.type === 'tool_call') {
    return true;
  }
  
  // Check for legacy format tool calls (more precise detection)
  if (typeof parsedContent === 'string' && 
      parsedContent.trim().startsWith('tool_use\n{') && 
      parsedContent.includes('/tool_use')) {
    return true;
  }
  
  return false;
};

// Helper function to extract legacy tool call info
const extractLegacyToolCallInfo = (content) => {
  try {
    const toolUseRaw = content.replace(/\/?tool_use\s*:?/ig, '').trim();
    let functionName = "unknown";
    let parameters = {};
    let toolCallId = "";
    
    try {
      const toolUseData = JSON.parse(toolUseRaw);
      functionName = toolUseData?.function?.name || functionName;
      parameters = toolUseData?.function?.arguments || parameters;
      toolCallId = toolUseData?.tool_call_id || "";
    } catch (err) {
      console.error('Error parsing tool_use JSON:', err);
    }
    
    return { functionName, parameters, toolCallId };
  } catch (err) {
    console.error('Error extracting legacy tool call info:', err);
    return { functionName: "unknown", parameters: {}, toolCallId: "" };
  }
};

// Helper function to check if content is a tool response
const isToolResponse = (parsedContent) => {
  // Check for new format tool response
  if (parsedContent.parts && 
      Array.isArray(parsedContent.parts) && 
      parsedContent.parts[0]?.type === 'tool_response') {
    return true;
  }
  
  // Check for role: 'tool'
  if (parsedContent.role === 'tool') {
    return true;
  }
  
  return false;
};

// Helper function to create a message from a tool call
const createToolCallMessage = (messageId, parsedContent) => {  
  // Handle new format tool calls
  if (parsedContent.parts && 
      Array.isArray(parsedContent.parts) && 
      parsedContent.parts[0]?.type === 'tool_call') {
    
    const toolCall = parsedContent.parts[0].tool_call;
    const functionName = toolCall.function.name || "unknown";
    let parameters = {};
    
    try {
      if (typeof toolCall.function.arguments === 'string') {
        parameters = JSON.parse(toolCall.function.arguments);
      } else {
        parameters = toolCall.function.arguments || {};
      }
    } catch (err) {
      console.error('Error parsing tool call arguments:', err);
    }
    
    const result = {
      id: messageId,
      type: 'ai',
      content: `:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` called:::\n`,
      isComplete: true
    };
    
    return result;
  }
  
  // Handle legacy format tool calls
  if (typeof parsedContent === 'string' && 
      parsedContent.includes('tool_use')) {
    console.log("[createToolCallMessage] Processing legacy format tool call");
    
    const { functionName, parameters } = extractLegacyToolCallInfo(parsedContent);
    
    const result = {
      id: messageId,
      type: 'ai',
      content: `:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` called:::\n`,
      isComplete: true
    };
    
    return result;
  }
};

// Helper function to create a message based on role
const createMessageFromRole = (messageId, parsedContent, messageText) => {
  switch (parsedContent.role) {
    case 'human':
      return {
        id: messageId,
        type: 'user',
        content: messageText,
        isComplete: true
      };
    case 'ai':
      return {
        id: messageId,
        type: 'ai',
        content: messageText,
        isComplete: true
      };
    case 'system':
      const systemText = messageText.includes(':::system')
        ? messageText
        : `:::system ${messageText}:::`;
      return {
        id: messageId,
        type: 'system',
        content: systemText,
        isComplete: true
      };
    default:
      console.log('Unknown role:', parsedContent.role);
      return {
        id: messageId,
        type: "ai", // Default to AI type
        content: messageText,
        isComplete: true,
      };
  }
};

export const detectErrorType = (error) => {
  if (!error) return 'connection';
  const errorStr = error.toString().toLowerCase();

  if (errorStr.includes('failed to create message') ||
      errorStr.includes('llm') ||
      errorStr.includes('model') ||
      errorStr.includes('anthropic') ||
      errorStr.includes('openai')) {
    return 'llm_config';
  }

  if (errorStr.includes('connection') ||
      errorStr.includes('network') ||
      errorStr.includes('timeout')) {
    return 'connection';
  }

  return 'other';
};

export const generateTempId = () => `temp_${Math.floor(Math.random() * 1_000_000_000)}`;

export const createSystemMessage = (content, errorType = null) => {
  const messageContent = content.includes(':::system')
    ? content
    : `:::system ${content}:::`;
    
  return {
    id: generateTempId(),
    type: 'system',
    content: messageContent,
    ...(errorType && { errorType }),
    isComplete: true
  };
};
