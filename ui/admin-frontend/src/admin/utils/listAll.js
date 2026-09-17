// Picker and lookup fetches need every row of a list endpoint, but the list
// endpoints page (page_size defaults to 10) and `all=true` is unbounded: one
// request loads the whole table. listAll fetches the pages instead, so each
// request is bounded and the total is capped.

/** Rows per request. */
export const LIST_ALL_PAGE_SIZE = 100;

/** Most rows a picker loads; past this the list needs a search box, not a longer fetch. */
export const LIST_ALL_MAX_ROWS = 1000;

const MAX_PAGES = Math.ceil(LIST_ALL_MAX_ROWS / LIST_ALL_PAGE_SIZE);

// Most list endpoints reply { data: [...] }; a few (filters) reply a bare array.
const rowsOf = (payload) => (Array.isArray(payload) ? payload : payload?.data || []);

// A capped list is announced once in the layout (ListTruncatedToaster), the
// way denied mutations are, so no picker has to own a warning of its own.
const truncationListeners = new Set();

export const subscribeListTruncated = (fn) => {
  truncationListeners.add(fn);
  return () => truncationListeners.delete(fn);
};

const emitListTruncated = (detail) => {
  truncationListeners.forEach((fn) => {
    try {
      fn(detail);
    } catch (e) {
      console.error('list truncated listener failed', e);
    }
  });
};

/**
 * Fetches every page of a list endpoint, up to LIST_ALL_MAX_ROWS.
 *
 * The common case (up to LIST_ALL_PAGE_SIZE rows) is one request. For longer
 * lists the first reply's X-Total-Pages header tells how many pages remain,
 * and they are requested together, so a long list costs two round trips
 * rather than one per page. An endpoint that sends no paging headers is
 * walked page by page until a short page.
 *
 * Resolves to a response shaped like a single page of the same endpoint
 * (`{ data: { data: rows } }`, or `{ data: rows }` for bare-array endpoints)
 * holding all the rows, so callers read it the way they read one page.
 * `truncated` is true when the cap cut the list short.
 */
export const listAll = async (client, path, params = {}) => {
  const fetchPage = (page) =>
    client.get(path, { params: { ...params, page, page_size: LIST_ALL_PAGE_SIZE } });

  const first = await fetchPage(1);
  const bare = Array.isArray(first?.data);
  const headers = first?.headers || {};
  const rows = [...rowsOf(first?.data)];
  const totalPages = parseInt(headers['x-total-pages'] || '0', 10);
  let truncated = false;

  if (rows.length >= LIST_ALL_PAGE_SIZE) {
    if (totalPages > 0) {
      truncated = totalPages > MAX_PAGES;
      const lastPage = Math.min(totalPages, MAX_PAGES);
      const pages = [];
      for (let page = 2; page <= lastPage; page += 1) pages.push(page);
      const rest = await Promise.all(pages.map(fetchPage));
      rest.forEach((response) => rows.push(...rowsOf(response?.data)));
    } else {
      for (let page = 2; ; page += 1) {
        if (page > MAX_PAGES) {
          truncated = true;
          break;
        }
        const pageRows = rowsOf((await fetchPage(page))?.data);
        rows.push(...pageRows);
        if (pageRows.length < LIST_ALL_PAGE_SIZE) break;
      }
    }
  }

  if (truncated) {
    console.warn(`listAll: ${path} has more than ${LIST_ALL_MAX_ROWS} rows; showing the first ${LIST_ALL_MAX_ROWS}`);
    emitListTruncated({ path, limit: LIST_ALL_MAX_ROWS });
  }

  const capped = rows.slice(0, LIST_ALL_MAX_ROWS);
  return { data: bare ? capped : { data: capped }, headers, truncated };
};

export default listAll;
