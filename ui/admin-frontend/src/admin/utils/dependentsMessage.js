import apiClient from "./apiClient";

// Shared wording for the delete confirmations on the LLM, tool, data source,
// secret, filter and model router lists. The dependents endpoint returns the
// same shape for every type, so the sentence is built in one place.

// Order matters: it is the order the groups are listed in the sentence and
// in the "Used by" section on detail pages. `label` is the row heading there
// and `path` the admin page an item links to (`/admin/<path>/<id>`).
export const DEPENDENT_GROUPS = [
  { key: "apps", singular: "app", plural: "apps", label: "Apps", path: "apps" },
  { key: "catalogues", singular: "catalog", plural: "catalogs", label: "Catalogs", path: "catalogs/llms" },
  { key: "llms", singular: "LLM provider", plural: "LLM providers", label: "LLM providers", path: "llms" },
  { key: "tools", singular: "tool", plural: "tools", label: "Tools", path: "tools" },
  { key: "datasources", singular: "data source", plural: "data sources", label: "Data sources", path: "datasources" },
  { key: "agents", singular: "agent", plural: "agents", label: "Agents", path: "agents" },
  { key: "model_routers", singular: "model router", plural: "model routers", label: "Model routers", path: "model-routers" },
  { key: "chats", singular: "chat", plural: "chats", label: "Chats", path: "chats" },
];

// The `catalogues` group means a different catalogue type depending on what
// was asked about: a tool sits in tool catalogues, a data source in data
// catalogues, everything else in LLM catalogues.
const CATALOGUE_PATH_BY_RESOURCE = {
  tools: "catalogs/tools",
  datasources: "catalogs/data",
};

/**
 * The non-empty dependent groups of an object, in display order, each with
 * its label, count, items and the admin path its items link to. Shared by
 * the delete confirmation and the "Used by" section so they never disagree
 * on ordering or wording.
 * @param {object|null} dependents - attributes from the dependents endpoint
 * @param {{resourcePath?: string}} [options] - the collection the dependents
 *   were fetched for, used to pick the catalogue link target
 */
export const groupsForDependents = (dependents, { resourcePath } = {}) => {
  if (!dependents) return [];
  return DEPENDENT_GROUPS.filter(
    (group) => Array.isArray(dependents[group.key]) && dependents[group.key].length > 0,
  ).map((group) => {
    const items = dependents[group.key];
    const path =
      group.key === "catalogues" ? CATALOGUE_PATH_BY_RESOURCE[resourcePath] || group.path : group.path;
    return {
      key: group.key,
      label: group.label,
      count: items.length,
      items: items.map((item) => ({ ...item, href: `/admin/${path}/${item.id}` })),
    };
  });
};

const MAX_NAMES = 5;

const joinNames = (parts) => {
  if (parts.length <= 1) return parts.join("");
  return `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;
};

const describeGroup = ({ singular, plural }, items) => {
  const count = items.length;
  const noun = count === 1 ? singular : plural;
  const names = items
    .map((item) => item?.name)
    .filter(Boolean)
    .slice(0, MAX_NAMES);
  const overflow = count - names.length;
  const listed =
    names.length === 0
      ? ""
      : ` (${names.join(", ")}${overflow > 0 ? `, +${overflow} more` : ""})`;
  return `${count} ${noun}${listed}`;
};

/**
 * Fetches the dependents of an object.
 * @param {string} resourcePath - API collection, e.g. "llms" or "model-routers"
 * @param {string|number} id
 * @returns {Promise<object|null>} the `attributes` block, or null when the
 *   request fails (the caller falls back to a generic sentence).
 */
export const fetchDependents = async (resourcePath, id) => {
  try {
    const response = await apiClient.get(`/${resourcePath}/${id}/dependents`);
    return response.data?.data?.attributes || null;
  } catch (error) {
    console.error(`Error fetching dependents for ${resourcePath}/${id}`, error);
    return null;
  }
};

/**
 * Builds the consequence sentence shown in the delete confirmation.
 * @param {object|null} dependents - attributes from the dependents endpoint
 * @param {string} objectLabel - lower-case noun, e.g. "LLM", "tool"
 * @param {string} [consequence] - what happens to the dependents on delete
 */
export const buildDependentsMessage = (
  dependents,
  objectLabel,
  consequence = "Deleting it removes it from all of them.",
) => {
  if (!dependents) {
    return `Deleting this ${objectLabel} removes it from everything that references it.`;
  }

  const groups = groupsForDependents(dependents);
  const total =
    typeof dependents.total === "number"
      ? dependents.total
      : groups.reduce((sum, group) => sum + group.count, 0);

  if (total === 0 || groups.length === 0) {
    return `Nothing references this ${objectLabel}.`;
  }

  const byKey = Object.fromEntries(DEPENDENT_GROUPS.map((g) => [g.key, g]));
  const parts = groups.map((group) => describeGroup(byKey[group.key], group.items));
  return `Used by ${joinNames(parts)}. ${consequence}`;
};

export default buildDependentsMessage;
