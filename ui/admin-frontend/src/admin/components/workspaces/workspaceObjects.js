/**
 * The object types GET /workspaces/:id/objects counts (the backend's
 * workspaces.CountedResources), with their labels and the admin list each
 * links to, pre-filtered with ?workspace_id=. Filestores have no list page.
 */
export const WORKSPACE_OBJECT_TYPES = [
  { key: 'llms', label: 'Model providers', path: '/admin/llms' },
  { key: 'apps', label: 'Apps', path: '/admin/apps' },
  { key: 'agents', label: 'Agents', path: '/admin/agents' },
  { key: 'tools', label: 'Tools', path: '/admin/tools' },
  { key: 'datasources', label: 'Data sources', path: '/admin/datasources' },
  { key: 'embedders', label: 'Embedders', path: '/admin/embedders' },
  { key: 'filters', label: 'Filters', path: '/admin/filters' },
  { key: 'secrets', label: 'Secrets', path: '/admin/secrets' },
  { key: 'mcp-servers', label: 'MCP servers', path: '/admin/mcp-servers' },
  { key: 'model-routers', label: 'Model routers', path: '/admin/model-routers' },
  { key: 'semantic-routers', label: 'Semantic routers', path: '/admin/semantic-routers' },
  { key: 'catalogues', label: 'Model provider catalogs', path: '/admin/catalogs/llms' },
  { key: 'data-catalogues', label: 'Data source catalogs', path: '/admin/catalogs/datasources' },
  { key: 'tool-catalogues', label: 'Tool catalogs', path: '/admin/catalogs/tools' },
  { key: 'filestores', label: 'Files' },
];

/** The admin list of a type, filtered to one Workspace (undefined: no list). */
export const workspaceListPath = (type, workspaceId) =>
  type.path ? `${type.path}?workspace_id=${workspaceId}` : undefined;

/** Sum of the counts. */
export const totalObjects = (counts) => Object.values(counts || {}).reduce((sum, n) => sum + (Number(n) || 0), 0);
