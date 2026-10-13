/**
 * Lets the API client tell the sync-status provider that something which the
 * edge gateways care about has just changed.
 *
 * Sync status polls every 30 seconds, so after saving a filter or a provider
 * the "Configuration Sync Pending" banner could take half a minute to appear.
 * That half minute is exactly the window in which someone saves a change,
 * calls the gateway, sees the old behaviour and concludes the product is
 * broken -- the most common shape of "I configured it and nothing happened".
 *
 * A module-level registration keeps the API client from importing React
 * context, and keeps every form from having to remember to do this itself.
 */

// Resources whose changes have to be pushed to the edge before they take
// effect. Matched against the request path.
const GATEWAY_AFFECTING = [
  /^\/?llms(\/|$)/,
  /^\/?filters(\/|$)/,
  /^\/?tools(\/|$)/,
  /^\/?apps(\/|$)/,
  /^\/?datasources(\/|$)/,
  /^\/?plugins(\/|$)/,
  /^\/?catalogues(\/|$)/,
  /^\/?tool-catalogues(\/|$)/,
  /^\/?data-catalogues(\/|$)/,
  /^\/?metadata\/objects(\/|$)/,
];

const MUTATING = ["post", "patch", "put", "delete"];

let refresh = null;
let scheduled = null;

export const registerSyncStatusRefresh = (fn) => {
  refresh = fn;
  return () => {
    if (refresh === fn) refresh = null;
  };
};

export const affectsGatewayConfig = (method, url) => {
  if (!method || !url) return false;
  if (!MUTATING.includes(method.toLowerCase())) return false;

  // Pushing configuration is not itself a config change.
  if (/\/edges(\/|$)/.test(url)) return false;

  const path = url.split("?")[0];
  return GATEWAY_AFFECTING.some((re) => re.test(path));
};

// Requests to a gateway-affecting resource that do not save it: a filter
// test run and a plugin's own RPC calls. They still refresh sync status, but
// they are not "you just saved this" moments, so they raise no push reminder.
const NOT_A_SAVE = [/^\/?filters\/test$/, /^\/?plugins\/[^/]+\/rpc(\/|$)/];

/**
 * isConfigSave reports whether a request saved (created, changed or deleted)
 * something the edge gateways consume: the same resource list as
 * affectsGatewayConfig, minus the non-saving actions above. The UI uses it
 * to remind the user, right after the save, that edges need a push (#506).
 */
export const isConfigSave = (method, url) => {
  if (!affectsGatewayConfig(method, url)) return false;
  const path = url.split("?")[0];
  return !NOT_A_SAVE.some((re) => re.test(path));
};

let savedInBurst = false;

/**
 * Coalesces bursts of saves (a catalog form issues one request per member)
 * into a single refresh shortly after the last one. The refresh is told
 * whether any request in the burst was a save ({ saved: true }).
 */
export const notifyConfigChanged = ({ saved = false } = {}) => {
  if (!refresh) return;
  savedInBurst = savedInBurst || saved;
  if (scheduled) clearTimeout(scheduled);
  scheduled = setTimeout(() => {
    scheduled = null;
    const fn = refresh;
    const options = { saved: savedInBurst };
    savedInBurst = false;
    if (fn) fn(options);
  }, 500);
};

export const __resetForTests = () => {
  refresh = null;
  if (scheduled) clearTimeout(scheduled);
  scheduled = null;
  savedInBurst = false;
};
