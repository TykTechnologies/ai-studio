import { useEffect, useState } from "react";
import pubClient from "../../admin/utils/pubClient";

/**
 * Loads the caller's unified catalog (GET /common/catalog): every LLM
 * provider, data source, tool and plugin resource they can build with, plus
 * the counts, catalogs and resource types the browse page filters on.
 */
const usePortalCatalog = () => {
  const [state, setState] = useState({ items: [], meta: null, loading: true, error: null });

  useEffect(() => {
    let cancelled = false;
    pubClient
      .get("/common/catalog")
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
  }, []);

  return state;
};

export default usePortalCatalog;
