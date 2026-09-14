import { useEffect, useMemo, useState } from "react";
import pubClient from "../../admin/utils/pubClient";

const EMPTY = { items: [], meta: null, loading: true, error: null };

/**
 * Loads one page of the caller's unified catalog (GET /common/catalog).
 * Search, filters, sort and paging are query parameters and are applied on
 * the server; `meta` carries the page position and the facets (counts per
 * type, kinds, catalogs, resource types) for the filter controls. The
 * request is re-issued whenever the parameters change.
 *
 * @param {object} params q, type, kind, privacy, catalog, community, sort,
 *   page, page_size (empty values are dropped)
 */
const usePortalCatalog = (params = {}) => {
  const [state, setState] = useState(EMPTY);

  // A stable key so an equal object on the next render does not refetch.
  const key = useMemo(() => {
    const clean = {};
    Object.keys(params)
      .sort()
      .forEach((name) => {
        const value = params[name];
        if (value === "" || value === null || value === undefined || value === false) return;
        clean[name] = value === true ? "true" : String(value);
      });
    return JSON.stringify(clean);
  }, [params]);

  useEffect(() => {
    let cancelled = false;
    setState((current) => ({ ...current, loading: true, error: null }));
    pubClient
      .get("/common/catalog", { params: JSON.parse(key) })
      .then((response) => {
        if (cancelled) return;
        setState({
          items: response.data?.data || [],
          meta: response.data?.meta || null,
          loading: false,
          error: null,
        });
      })
      .catch((error) => {
        console.error("Error fetching the portal catalog:", error);
        if (cancelled) return;
        setState({ items: [], meta: null, loading: false, error });
      });
    return () => {
      cancelled = true;
    };
  }, [key]);

  return state;
};

export default usePortalCatalog;
