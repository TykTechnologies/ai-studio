import { useState, useEffect, useMemo } from 'react';
import { getPermissionCatalogue } from '../services/rbacService';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';
import { permissionLabel } from '../rbac/permissions';

const CATALOGUE_TTL_MS = 5 * 60 * 1000;

/** Drops the cached catalogue so the next hook instance refetches it. */
export const invalidatePermissionCatalogue = () => {
  cacheService.remove(CACHE_KEYS.RBAC_CATALOGUE);
};

/**
 * Loads the permission catalogue (resources, actions, grouping) that the role
 * editor and permission lists render. Built-in resources only change on
 * upgrade, but plugins add and remove their own resources at runtime, so the
 * cache is short-lived and dropped whenever the plugin loader refreshes.
 */
const usePermissionCatalogue = () => {
  const [catalogue, setCatalogue] = useState(() => cacheService.get(CACHE_KEYS.RBAC_CATALOGUE) || null);
  const [loading, setLoading] = useState(!catalogue);
  const [error, setError] = useState(null);

  useEffect(() => {
    const onPluginsChanged = () => {
      invalidatePermissionCatalogue();
      setCatalogue(null);
    };
    window.addEventListener('plugin-loader-refreshed', onPluginsChanged);
    return () => window.removeEventListener('plugin-loader-refreshed', onPluginsChanged);
  }, []);

  useEffect(() => {
    if (catalogue) return;
    let cancelled = false;
    getPermissionCatalogue()
      .then((data) => {
        if (cancelled) return;
        cacheService.set(CACHE_KEYS.RBAC_CATALOGUE, data, CATALOGUE_TTL_MS);
        setCatalogue(data);
      })
      .catch((err) => {
        if (!cancelled) setError(err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [catalogue]);

  const derived = useMemo(() => {
    const resources = catalogue?.resources || [];
    const groups = catalogue?.groups || [];
    const actions = catalogue?.actions || ['read', 'write', 'delete', 'execute', 'publish'];
    const byKey = new Map(resources.map((r) => [r.key, r]));
    // Within a group, plugin-contributed resources are rendered as one
    // sub-table per plugin after the built-ins.
    const grouped = groups
      .map((group) => {
        const inGroup = resources.filter((r) => r.group === group);
        const builtIn = inGroup.filter((r) => !r.plugin);
        const plugins = [];
        inGroup
          .filter((r) => r.plugin)
          .forEach((r) => {
            let entry = plugins.find((p) => p.plugin === r.plugin);
            if (!entry) {
              entry = { plugin: r.plugin, label: r.plugin_label || r.plugin, resources: [] };
              plugins.push(entry);
            }
            entry.resources.push(r);
          });
        return { group, resources: inGroup, builtIn, plugins };
      })
      .filter((g) => g.resources.length > 0);
    const label = (perm) => {
      if (!perm || perm === '*') return permissionLabel(perm);
      const i = perm.lastIndexOf(':');
      const res = byKey.get(perm.slice(0, i));
      const act = perm.slice(i + 1);
      if (!res) return permissionLabel(perm);
      return `${res.label}: ${act}`;
    };
    return { resources, groups, actions, byKey, grouped, label, enabled: catalogue?.enabled === true };
  }, [catalogue]);

  return { ...derived, catalogue, loading, error };
};

export default usePermissionCatalogue;
