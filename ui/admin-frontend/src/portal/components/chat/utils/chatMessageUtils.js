export const parseServerMessage = (msg) => {
  try {
    const content = msg.attributes?.content || msg.content;
    const messageId = msg.id;
    
    if (!messageId) {
      console.error('Message ID missing from server response:', msg);
      return null;
    }
    
    const parsedContent = JSON.parse(content);

    // Handle different message roles
    const messageContent = parsedContent.context
      ? `[CONTEXT]${parsedContent.context}[/CONTEXT]${parsedContent.text}`
      : parsedContent.text;

    switch (parsedContent.role) {
      case 'human':
        return {
          id: messageId,
          type: 'user',
          content: messageContent,
          isComplete: true
        };
      case 'ai':
        return {
          id: messageId,
          type: 'ai',
          content: messageContent,
          isComplete: true
        };
      case 'system':
        const systemText = parsedContent.text.includes(':::system')
          ? messageContent
          : `:::system ${messageContent}:::`;
        return {
          id: messageId,
          type: 'system',
          content: systemText,
          isComplete: true
        };
      case 'tool':
        return {
          id: messageId,
          type: 'ai',
          content: messageContent,
          isComplete: true
        };
      default:
        console.log('Unknown role:', parsedContent.role);
        return null;
    }
  } catch (e) {
    // If parsing fails, treat it as an AI message with direct content
    return {
      id: msg.id,
      type: "ai",
      content: msg.attributes?.content || msg.content,
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
