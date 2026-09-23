// A pool pattern is a glob when it contains *, ? or [; anything else names
// one model outright.
const isGlob = (pattern) => /[*?[]/.test(pattern);

/**
 * The model strings an App sends on the unified endpoint to reach this
 * router: "<router-slug>/<model>" for every literal (non-glob) pool pattern
 * and every mapping source model. Patterns may be comma-separated. Globs are
 * left out: they name no model a client could copy.
 */
export const routerModelStrings = (slug, pools = []) => {
  const models = [];
  const add = (model) => {
    const name = (model || "").trim();
    if (name && !isGlob(name) && !models.includes(name)) models.push(name);
  };
  (pools || []).forEach((pool) => {
    String(pool?.model_pattern || "")
      .split(",")
      .forEach(add);
    (pool?.vendors || []).forEach((vendor) =>
      (vendor?.mappings || []).forEach((mapping) => add(mapping?.source_model)),
    );
  });
  return models.map((model) => `${slug}/${model}`);
};

export default routerModelStrings;
