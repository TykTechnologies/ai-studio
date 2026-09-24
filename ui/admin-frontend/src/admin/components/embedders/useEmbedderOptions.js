import { useEffect, useState } from "react";
import apiClient from "../../utils/apiClient";
import { listAll } from "../../utils/listAll";

/**
 * What the embedder fields choose from: LLM providers (for a linked
 * embedder) and the vendors that can embed. Either list comes back empty when
 * the caller may not read it; the fields still render.
 */
const useEmbedderOptions = (enabled = true) => {
  const [llms, setLLMs] = useState([]);
  const [vendors, setVendors] = useState([]);

  useEffect(() => {
    if (!enabled) return undefined;
    let cancelled = false;
    listAll(apiClient, "/llms")
      .then((response) => !cancelled && setLLMs(response.data.data || []))
      .catch(() => !cancelled && setLLMs([]));
    apiClient
      .get("/embedders/vendors")
      .then((response) => !cancelled && setVendors(response.data.data || []))
      .catch(() => !cancelled && setVendors([]));
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  return { llms, vendors };
};

export default useEmbedderOptions;
