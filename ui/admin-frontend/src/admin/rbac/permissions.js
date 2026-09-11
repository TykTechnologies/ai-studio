/**
 * Permission vocabulary shared with the backend catalogue (pkg/authz).
 *
 * A permission is "<resource>:<action>" with actions read, write, delete and
 * execute. write, delete and execute each imply read. The single wildcard
 * "*" is held by full administrators (Owner / Administrator system roles).
 *
 * Everything in the UI references these constants so a rename is a
 * one-file change; the backend's GET /api/v1/rbac/permissions is the source
 * of truth for labels and grouping when rendering the role editor.
 */

export const FULL_ADMIN = '*';

export const ACTIONS = ['read', 'write', 'delete', 'execute'];

const p = (resource, action) => `${resource}:${action}`;

export const P = Object.freeze({
  // Analytics
  ANALYTICS_READ: p('analytics', 'read'),
  PROXY_LOGS_READ: p('proxy-logs', 'read'),

  // Plugins
  PLUGINS_READ: p('plugins', 'read'),
  PLUGINS_WRITE: p('plugins', 'write'),
  PLUGINS_DELETE: p('plugins', 'delete'),
  PLUGINS_EXECUTE: p('plugins', 'execute'),
  MARKETPLACE_READ: p('marketplace', 'read'),
  MARKETPLACE_WRITE: p('marketplace', 'write'),
  MARKETPLACE_DELETE: p('marketplace', 'delete'),
  MARKETPLACE_EXECUTE: p('marketplace', 'execute'),

  // LLM management
  LLMS_READ: p('llms', 'read'),
  LLMS_WRITE: p('llms', 'write'),
  LLMS_DELETE: p('llms', 'delete'),
  MODEL_PRICES_READ: p('model-prices', 'read'),
  MODEL_PRICES_WRITE: p('model-prices', 'write'),
  MODEL_PRICES_DELETE: p('model-prices', 'delete'),
  MODEL_ROUTERS_READ: p('model-routers', 'read'),
  MODEL_ROUTERS_WRITE: p('model-routers', 'write'),
  MODEL_ROUTERS_DELETE: p('model-routers', 'delete'),

  // Context management
  DATASOURCES_READ: p('datasources', 'read'),
  DATASOURCES_WRITE: p('datasources', 'write'),
  DATASOURCES_DELETE: p('datasources', 'delete'),
  DATASOURCES_EXECUTE: p('datasources', 'execute'),
  TOOLS_READ: p('tools', 'read'),
  TOOLS_WRITE: p('tools', 'write'),
  TOOLS_DELETE: p('tools', 'delete'),
  TOOLS_EXECUTE: p('tools', 'execute'),
  FILTERS_READ: p('filters', 'read'),
  FILTERS_WRITE: p('filters', 'write'),
  FILTERS_DELETE: p('filters', 'delete'),
  FILTERS_EXECUTE: p('filters', 'execute'),
  FILESTORES_READ: p('filestores', 'read'),
  FILESTORES_WRITE: p('filestores', 'write'),
  TAGS_READ: p('tags', 'read'),
  TAGS_WRITE: p('tags', 'write'),

  // Community
  SUBMISSIONS_READ: p('submissions', 'read'),
  SUBMISSIONS_WRITE: p('submissions', 'write'),
  SUBMISSIONS_EXECUTE: p('submissions', 'execute'),
  ATTESTATION_TEMPLATES_READ: p('attestation-templates', 'read'),
  ATTESTATION_TEMPLATES_WRITE: p('attestation-templates', 'write'),
  ATTESTATION_TEMPLATES_DELETE: p('attestation-templates', 'delete'),

  // Access
  USERS_READ: p('users', 'read'),
  USERS_WRITE: p('users', 'write'),
  USERS_DELETE: p('users', 'delete'),
  GROUPS_READ: p('groups', 'read'),
  GROUPS_WRITE: p('groups', 'write'),
  GROUPS_DELETE: p('groups', 'delete'),
  ROLES_READ: p('roles', 'read'),
  ROLES_WRITE: p('roles', 'write'),
  ROLES_DELETE: p('roles', 'delete'),
  SSO_PROFILES_READ: p('sso-profiles', 'read'),
  SSO_PROFILES_WRITE: p('sso-profiles', 'write'),
  SSO_PROFILES_DELETE: p('sso-profiles', 'delete'),

  // Governance
  AUDIT_READ: p('audit', 'read'),
  COMPLIANCE_READ: p('compliance', 'read'),
  METADATA_READ: p('metadata', 'read'),
  METADATA_WRITE: p('metadata', 'write'),
  METADATA_DELETE: p('metadata', 'delete'),
  EXPORTS_READ: p('exports', 'read'),
  EXPORTS_WRITE: p('exports', 'write'),
  WEBHOOKS_READ: p('webhooks', 'read'),
  WEBHOOKS_WRITE: p('webhooks', 'write'),
  WEBHOOKS_DELETE: p('webhooks', 'delete'),
  WEBHOOKS_EXECUTE: p('webhooks', 'execute'),

  // Settings
  SECRETS_READ: p('secrets', 'read'),
  SECRETS_WRITE: p('secrets', 'write'),
  SECRETS_DELETE: p('secrets', 'delete'),
  BRANDING_READ: p('branding', 'read'),
  BRANDING_WRITE: p('branding', 'write'),

  // AI Portal
  APPS_READ: p('apps', 'read'),
  APPS_WRITE: p('apps', 'write'),
  APPS_DELETE: p('apps', 'delete'),
  CREDENTIALS_READ: p('credentials', 'read'),
  CREDENTIALS_WRITE: p('credentials', 'write'),
  EDGES_READ: p('edges', 'read'),
  EDGES_WRITE: p('edges', 'write'),
  EDGES_DELETE: p('edges', 'delete'),
  EDGES_EXECUTE: p('edges', 'execute'),

  // Chat
  CHATS_READ: p('chats', 'read'),
  CHATS_WRITE: p('chats', 'write'),
  CHATS_DELETE: p('chats', 'delete'),
  AGENTS_READ: p('agents', 'read'),
  AGENTS_WRITE: p('agents', 'write'),
  AGENTS_DELETE: p('agents', 'delete'),
  AGENTS_EXECUTE: p('agents', 'execute'),
  LLM_SETTINGS_READ: p('llm-settings', 'read'),
  LLM_SETTINGS_WRITE: p('llm-settings', 'write'),
  LLM_SETTINGS_DELETE: p('llm-settings', 'delete'),
  CHAT_HISTORY_READ: p('chat-history', 'read'),
  CHAT_HISTORY_DELETE: p('chat-history', 'delete'),

  // Catalogs
  CATALOGUES_READ: p('catalogues', 'read'),
  CATALOGUES_WRITE: p('catalogues', 'write'),
  CATALOGUES_DELETE: p('catalogues', 'delete'),
  DATA_CATALOGUES_READ: p('data-catalogues', 'read'),
  DATA_CATALOGUES_WRITE: p('data-catalogues', 'write'),
  DATA_CATALOGUES_DELETE: p('data-catalogues', 'delete'),
  TOOL_CATALOGUES_READ: p('tool-catalogues', 'read'),
  TOOL_CATALOGUES_WRITE: p('tool-catalogues', 'write'),
  TOOL_CATALOGUES_DELETE: p('tool-catalogues', 'delete'),
});

/** Normalises a permission prop that may be a string or an array. */
export const toArray = (permOrArray) => {
  if (!permOrArray) return [];
  return Array.isArray(permOrArray) ? permOrArray : [permOrArray];
};

export const resourceOf = (perm) => {
  const i = typeof perm === 'string' ? perm.lastIndexOf(':') : -1;
  return i > 0 ? perm.slice(0, i) : '';
};

export const actionOf = (perm) => {
  const i = typeof perm === 'string' ? perm.lastIndexOf(':') : -1;
  return i > 0 ? perm.slice(i + 1) : '';
};

/**
 * Evaluates a permission against a set of held permission strings with the
 * same semantics as the backend: the wildcard grants everything, and any
 * action on a resource satisfies a read check for that resource.
 */
export const hasPermission = (held, perm) => {
  if (!perm) return true;
  if (!held || held.size === 0) return false;
  if (held.has(FULL_ADMIN)) return true;
  if (held.has(perm)) return true;
  if (actionOf(perm) === 'read') {
    const res = resourceOf(perm);
    for (const h of held) {
      if (resourceOf(h) === res) return true;
    }
  }
  return false;
};

/** Human label for a permission when the catalogue is not at hand. */
export const permissionLabel = (perm) => {
  if (!perm) return '';
  if (perm === FULL_ADMIN) return 'Full administrator access';
  const res = resourceOf(perm).replace(/-/g, ' ');
  const act = actionOf(perm);
  const resLabel = res.charAt(0).toUpperCase() + res.slice(1);
  return `${resLabel}: ${act}`;
};
