export const reorderAndMergeToolMessages = (messages, tools) => {
  console.log("Reordering and merging tool messages");
  const result = [...messages];

  for (let i = 0; i < result.length; i++) {
    if (result[i]?.type === 'ai' && result[i]?.content.includes('tool_use')) {
      if (
        i + 2 < result.length &&
        result[i + 1]?.type === 'ai' && result[i + 1]?.content.includes('tool_result') &&
        result[i + 2]?.type === 'ai' &&
        !result[i + 2]?.content.includes('tool_use') &&
        !result[i + 2]?.content.includes('tool_result')
      ) {
        const explanation = result.splice(i + 2, 1)[0];
        result.splice(i, 0, explanation);
        i += 2;
      }
    }
  }

  for (let i = 0; i < result.length; i++) {
    const current = result[i];
    if (
      current?.type === 'ai' &&
      !current.content.includes('tool_use') &&
      !current.content.includes('tool_result')
    ) {
      if (
        i + 2 < result.length &&
        result[i + 1]?.type === 'ai' && result[i + 1].content.includes('tool_use') &&
        result[i + 2]?.type === 'ai' && result[i + 2].content.includes('tool_result')
      ) {
        const toolUseRaw = result[i + 1].content.replace(/\/?tool_use\s*:?/ig, '').trim();
        let functionName = "unknown";
        let parameters = {};
        try {
          const toolUseData = JSON.parse(toolUseRaw);
          functionName = toolUseData?.function?.name || functionName;
          parameters = toolUseData?.function?.arguments || parameters;
        } catch (err) {
          console.error('Error parsing tool_use JSON:', err);
        }

        const toolResultRaw = result[i + 2].content.replace(/\/?tool_result\s*:?/ig, '').trim();
        let contentData = {};
        try {
          const toolResultData = JSON.parse(toolResultRaw);
          contentData = toolResultData?.content || contentData;
        } catch (err) {
          console.error('Error parsing tool_result JSON:', err);
        }

        const contentString = JSON.stringify(contentData);
        let byteCount = 0;
        try {
          byteCount = new TextEncoder().encode(contentString).length;
        } catch (err) {
          byteCount = contentString.length;
        }

        const systemMsg = contentString && contentString.trim() 
          ? `\n:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` returned: \`${byteCount}\` bytes:::\n
[CONTEXT]${contentString}[/CONTEXT]\n`
          : `\n:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` returned: \`${byteCount}\` bytes:::\n`;

        current.content += systemMsg;
        result.splice(i + 1, 2);
      }
    }
  }

  return result;
};

export const processToolsAndDatasources = (data) => {
  // Modify the data object directly to match the original implementation
  if (data.tools && Array.isArray(data.tools)) {
    data.tools.forEach(tool => {
      const uniqueId = `tool-${tool.id}`;
      tool.type = 'tool';
      tool.uniqueId = uniqueId;
    });
  }
  
  if (data.datasources && Array.isArray(data.datasources)) {
    data.datasources.forEach(ds => {
      const uniqueId = `database-${ds.id}`;
      ds.type = 'database';
      ds.uniqueId = uniqueId;
    });
  }
  
  return data;
};
