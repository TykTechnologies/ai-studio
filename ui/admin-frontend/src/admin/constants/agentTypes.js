/**
 * Agent message chunk types from backend SSE stream
 */
export const AGENT_CHUNK_TYPES = {
  CONTENT: 'CONTENT',
  TOOL_CALL: 'TOOL_CALL',
  TOOL_RESULT: 'TOOL_RESULT',
  THINKING: 'THINKING',
  ERROR: 'ERROR',
  DONE: 'DONE',
};

/**
 * Agent message chunk type labels for display
 */
export const AGENT_CHUNK_TYPE_LABELS = {
  [AGENT_CHUNK_TYPES.CONTENT]: 'Response',
  [AGENT_CHUNK_TYPES.TOOL_CALL]: 'Tool Call',
  [AGENT_CHUNK_TYPES.TOOL_RESULT]: 'Tool Result',
  [AGENT_CHUNK_TYPES.THINKING]: 'Thinking',
  [AGENT_CHUNK_TYPES.ERROR]: 'Error',
  [AGENT_CHUNK_TYPES.DONE]: 'Completed',
};

/**
 * Plugin types that support agent functionality
 */
export const PLUGIN_TYPES = {
  GATEWAY: 'gateway',
  AI_STUDIO: 'ai_studio',
  AGENT: 'agent',
};

/**
 * Check if a plugin is an agent plugin
 */
export const isAgentPlugin = (plugin) => {
  return plugin?.pluginType === PLUGIN_TYPES.AGENT;
};

/**
 * Agent status types
 */
export const AGENT_STATUS = {
  ACTIVE: 'active',
  INACTIVE: 'inactive',
};

export default {
  AGENT_CHUNK_TYPES,
  AGENT_CHUNK_TYPE_LABELS,
  PLUGIN_TYPES,
  AGENT_STATUS,
  isAgentPlugin,
};
