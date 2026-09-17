// Picker and lookup fetches need every row of a list endpoint, but the list
// endpoints page (page_size defaults to 10) and `all=true` is unbounded: one
// request loads the whole table. listAll walks the pages instead, so each
// request is bounded and the total is capped.

/** Rows per request. */
export const LIST_ALL_PAGE_SIZE = 100;

/** Most rows a picker loads; past this the list needs a search box, not a longer fetch. */
export const LIST_ALL_MAX_ROWS = 1000;

// Most list endpoints reply { data: [...] }; a few (filters) reply a bare array.
const rowsOf = (payload) => (Array.isArray(payload) ? payload : payload?.data || []);

/**
 * Fetches every page of a list endpoint, up to LIST_ALL_MAX_ROWS.
 *
 * Resolves to a response shaped like a single page of the same endpoint
 * (`{ data: { data: rows } }`, or `{ data: rows }` for bare-array endpoints)
 * holding all the rows, so callers read it the way they read one page.
 * `truncated` is true when the cap cut the list short.
 */
export const listAll = async (client, path, params = {}) => {
  const rows = [];
  let bare = false;
  let truncated = false;
  let headers = {};

  for (let page = 1; ; page += 1) {
    const response = await client.get(path, {
      params: { ...params, page, page_size: LIST_ALL_PAGE_SIZE },
    });
    const payload = response?.data;
    const pageRows = rowsOf(payload);
    if (page === 1) {
      bare = Array.isArray(payload);
      headers = response?.headers || {};
    }
    rows.push(...pageRows);

    const totalPages = parseInt(response?.headers?.['x-total-pages'] || '0', 10);
    if (pageRows.length < LIST_ALL_PAGE_SIZE || (totalPages > 0 && page >= totalPages)) break;
    if (rows.length >= LIST_ALL_MAX_ROWS) {
      truncated = true;
      // eslint-disable-next-line no-console
      console.warn(`listAll: ${path} has more than ${LIST_ALL_MAX_ROWS} rows; showing the first ${LIST_ALL_MAX_ROWS}`);
      break;
    }
  }

  const capped = rows.slice(0, LIST_ALL_MAX_ROWS);
  return { data: bare ? capped : { data: capped }, headers, truncated };
};

export default listAll;
