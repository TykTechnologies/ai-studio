import apiClient from "../../utils/apiClient";

/**
 * Bulk operations on list pages.
 *
 * Resources with a server-side endpoint take one call:
 *   POST /api/v1/{resource}/bulk { action, ids }
 *   -> { data: { action, results: [{ id, ok, error }], succeeded, failed } }
 * Everything else (catalogues, users, roles, model prices) only supports
 * delete, done one request per item so the summary has the same shape.
 */

export const BULK_ENDPOINT_RESOURCES = new Set([
  "llms",
  "tools",
  "datasources",
  "apps",
  "filters",
  "secrets",
  "model-routers",
]);

export const BULK_ACTION_VERBS = {
  delete: { past: "Deleted", present: "delete" },
  activate: { past: "Activated", present: "activate" },
  deactivate: { past: "Deactivated", present: "deactivate" },
};

export const errorDetail = (error) =>
  error?.response?.data?.errors?.[0]?.detail ||
  error?.response?.data?.error ||
  error?.message ||
  "Unknown error";

// The API binds ids as unsigned integers; list rows carry JSON:API string
// ids ("42"), so numeric strings are sent as numbers.
export const normaliseId = (id) => (typeof id === "string" && /^\d+$/.test(id) ? Number(id) : id);

const countResults = (results) => ({
  succeeded: results.filter((result) => result.ok).length,
  failed: results.filter((result) => !result.ok).length,
});

/**
 * Runs one bulk action.
 * @param {object} options
 * @param {string} options.resource - API collection, e.g. "llms"
 * @param {"delete"|"activate"|"deactivate"} options.action
 * @param {Array<string|number>} options.ids
 * @param {boolean} [options.viaBulkEndpoint] - defaults to whether the
 *   resource has a /bulk endpoint
 * @param {(id) => Promise} [options.deleteOne] - per-item delete used when
 *   there is no bulk endpoint (defaults to DELETE /{resource}/{id})
 * @returns {Promise<{action, results, succeeded, failed}>}
 */
export const runBulkAction = async ({
  resource,
  action,
  ids,
  viaBulkEndpoint = BULK_ENDPOINT_RESOURCES.has(resource),
  deleteOne,
}) => {
  const targets = Array.isArray(ids) ? ids : [];
  if (viaBulkEndpoint) {
    const response = await apiClient.post(`/${resource}/bulk`, {
      action,
      ids: targets.map(normaliseId),
    });
    const payload = response?.data?.data || response?.data || {};
    const results = Array.isArray(payload.results) ? payload.results : [];
    const counts = countResults(results);
    return {
      action: payload.action || action,
      results,
      succeeded: typeof payload.succeeded === "number" ? payload.succeeded : counts.succeeded,
      failed: typeof payload.failed === "number" ? payload.failed : counts.failed,
    };
  }

  if (action !== "delete") {
    throw new Error(`Bulk ${action} is not available for ${resource}`);
  }
  const results = [];
  for (const id of targets) {
    try {
      // Sequential on purpose: these are mutations on shared data.
      // eslint-disable-next-line no-await-in-loop
      await (deleteOne ? deleteOne(id) : apiClient.delete(`/${resource}/${id}`));
      results.push({ id, ok: true });
    } catch (error) {
      results.push({ id, ok: false, error: errorDetail(error) });
    }
  }
  return { action, results, ...countResults(results) };
};

/**
 * Turns a bulk result into the one snackbar line plus the failures to list.
 * @param {{action, results, succeeded, failed}} result
 * @param {{ singular: string, plural: string, nameOf?: (id) => string, total?: number }} options
 * @returns {{ message: string, severity: "success"|"warning"|"error", failures: Array<{id, name, error}> }}
 */
export const summariseBulkResult = (result, { singular, plural, nameOf, total: totalOverride }) => {
  const verb = BULK_ACTION_VERBS[result?.action] || { past: "Processed", present: "process" };
  const results = Array.isArray(result?.results) ? result.results : [];
  const total = typeof totalOverride === "number" ? totalOverride : results.length;
  const succeeded = typeof result?.succeeded === "number" ? result.succeeded : 0;
  const failed = typeof result?.failed === "number" ? result.failed : Math.max(total - succeeded, 0);
  const noun = total === 1 ? singular : plural;

  const failures = results
    .filter((entry) => !entry.ok)
    .map((entry) => ({
      id: entry.id,
      name: (nameOf && nameOf(entry.id)) || `#${entry.id}`,
      error: entry.error || "Unknown error",
    }));

  if (failed === 0) {
    return { message: `${verb.past} ${total} ${noun}`, severity: "success", failures };
  }
  if (succeeded === 0) {
    return { message: `Failed to ${verb.present} ${total} ${noun}`, severity: "error", failures };
  }
  return {
    message: `${verb.past} ${succeeded} of ${total} ${noun}; ${failed} failed`,
    severity: "warning",
    failures,
  };
};
