import { useState, useEffect, useMemo } from 'react';
import { getPermissionCatalogue } from '../services/rbacService';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';
import { permissionLabel } from '../rbac/permissions';

const CATALOGUE_TTL_MS = 30 * 60 * 1000;

/**
 * Loads the permission catalogue (resources, actions, grouping) that the role
 * editor and permission lists render. Cached for half an hour: it only
 * changes on upgrade.
 */
const usePermissionCatalogue = () => {
  const [catalogue, setCatalogue] = useState(() => cacheService.get(CACHE_KEYS.RBAC_CATALOGUE) || null);
  const [loading, setLoading] = useState(!catalogue);
  const [error, setError] = useState(null);

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
    const actions = catalogue?.actions || ['read', 'write', 'delete', 'execute'];
    const byKey = new Map(resources.map((r) => [r.key, r]));
    const grouped = groups
      .map((group) => ({ group, resources: resources.filter((r) => r.group === group) }))
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
