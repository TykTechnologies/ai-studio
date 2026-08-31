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

/**
 * Coalesces bursts of saves (a catalog form issues one request per member)
 * into a single refresh shortly after the last one.
 */
export const notifyConfigChanged = () => {
  if (!refresh) return;
  if (scheduled) clearTimeout(scheduled);
  scheduled = setTimeout(() => {
    scheduled = null;
    const fn = refresh;
    if (fn) fn();
  }, 500);
};

export const __resetForTests = () => {
  refresh = null;
  if (scheduled) clearTimeout(scheduled);
  scheduled = null;
};
