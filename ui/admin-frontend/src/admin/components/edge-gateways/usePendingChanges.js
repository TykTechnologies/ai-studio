import { useState, useEffect, useMemo } from 'react';
import edgeGatewayService from '../../services/edgeGatewayService';

/**
 * Loads the pending-changes preview for one or more namespaces.
 *
 * Returns { byNamespace, loading, total, error } where byNamespace maps a
 * namespace to { loading, error, data } and `data` is what
 * edgeGatewayService.getPendingChanges resolved to. `total` is the sum of
 * every loaded namespace's real count (not the capped list length), and is
 * null until every namespace has loaded without error, so the caller can
 * tell "nothing to push" apart from "we do not know yet".
 *
 * @param {string[]} namespaces
 * @param {boolean} enabled - false skips fetching (e.g. modal closed)
 */
const usePendingChanges = (namespaces, enabled = true) => {
  const [byNamespace, setByNamespace] = useState({});
  // Stable identity for the list so a caller passing a fresh array each
  // render does not refetch every time.
  const key = JSON.stringify(namespaces || []);

  useEffect(() => {
    if (!enabled) return undefined;
    const list = JSON.parse(key);
    if (list.length === 0) {
      setByNamespace({});
      return undefined;
    }

    let cancelled = false;
    setByNamespace(
      Object.fromEntries(list.map((ns) => [ns, { loading: true, error: null, data: null }]))
    );

    list.forEach(async (ns) => {
      try {
        const data = await edgeGatewayService.getPendingChanges(ns);
        if (cancelled) return;
        setByNamespace((prev) => ({ ...prev, [ns]: { loading: false, error: null, data } }));
      } catch (err) {
        if (cancelled) return;
        setByNamespace((prev) => ({
          ...prev,
          [ns]: { loading: false, error: err.message || 'Failed to load pending changes', data: null },
        }));
      }
    });

    return () => {
      cancelled = true;
    };
  }, [key, enabled]);

  return useMemo(() => {
    const entries = Object.values(byNamespace);
    const loading = entries.some((e) => e.loading);
    const error = entries.find((e) => e.error)?.error || null;
    const complete = entries.length > 0 && entries.every((e) => !e.loading && !e.error && e.data);
    const total = complete ? entries.reduce((sum, e) => sum + (e.data.total || 0), 0) : null;
    return { byNamespace, loading, error, total };
  }, [byNamespace]);
};

export default usePendingChanges;
