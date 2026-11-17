import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { FormControl, InputLabel, MenuItem, Select, Typography } from "@mui/material";
import { usePermissions } from "../../context/PermissionsContext";
import { hasPermission } from "../../rbac/permissions";
import { listWorkspaces } from "../../services/workspacesService";

/**
 * Shared Workspace pieces for the object lists and forms (Workspaces E3/E4):
 * the names to show, a list column, a list filter bound to ?workspace_id=,
 * the move rule for edit forms, and the field errors the API answers with.
 * Everything renders nothing in Community Edition (rbacEnabled false).
 */

let namesPromise = null;

/** Forget the cached Workspace names (tests; after a Workspace is renamed). */
export const resetWorkspaceNames = () => {
  namesPromise = null;
};

const loadNames = () => {
  if (!namesPromise) {
    namesPromise = listWorkspaces()
      .then((list) => list.map((w) => ({ id: Number(w.id), name: w.attributes?.name || `Workspace ${w.id}` })))
      .catch(() => {
        namesPromise = null;
        return [];
      });
  }
  return namesPromise;
};

/**
 * useWorkspaces returns { enabled, workspaces: [{id, name}], nameOf(id) }:
 * the Workspaces the caller can see (every one for a global reader, else the
 * ones their roles are scoped to). A Workspace the caller has no access to
 * reads "Other Workspace": its name is not theirs to see.
 */
export const useWorkspaces = () => {
  const { rbacEnabled, workspaces: scoped } = usePermissions();
  const [listed, setListed] = useState(null);

  useEffect(() => {
    if (!rbacEnabled) return undefined;
    let live = true;
    loadNames().then((list) => {
      if (live) setListed(list);
    });
    return () => {
      live = false;
    };
  }, [rbacEnabled]);

  const workspaces = useMemo(() => {
    if (!rbacEnabled) return [];
    const byId = new Map();
    (scoped || []).forEach((w) => byId.set(Number(w.id), { id: Number(w.id), name: w.name || `Workspace ${w.id}` }));
    (listed || []).forEach((w) => byId.set(w.id, w));
    return [...byId.values()];
  }, [rbacEnabled, scoped, listed]);

  const nameOf = useCallback(
    (id) => {
      if (id === undefined || id === null || id === "") return "";
      const hit = workspaces.find((w) => w.id === Number(id));
      return hit ? hit.name : "Other Workspace";
    },
    [workspaces],
  );

  return { enabled: Boolean(rbacEnabled), workspaces, nameOf };
};

/**
 * useWorkspaceColumn returns the DataTable column showing an object's
 * Workspace, or null in Community Edition. getId reads the id from a row
 * (default: row.attributes.workspace_id).
 */
export const useWorkspaceColumn = (getId = (row) => row?.attributes?.workspace_id) => {
  const { enabled, nameOf } = useWorkspaces();
  return useMemo(
    () =>
      enabled
        ? {
            field: "workspace_id",
            headerName: "Workspace",
            renderCell: (row) => (
              <Typography variant="bodyMediumDefault" data-testid="workspace-cell">
                {nameOf(getId(row))}
              </Typography>
            ),
          }
        : null,
    [enabled, nameOf, getId],
  );
};

/**
 * useWorkspaceFilter keeps the list's Workspace filter in the URL
 * (?workspace_id=), so the Workspace page can link to a pre-filtered list.
 * It returns { workspaceId, setWorkspaceId, params } where params is merged
 * into the list request ({} when no filter is set).
 */
export const useWorkspaceFilter = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { enabled } = useWorkspaces();
  const raw = new URLSearchParams(location.search).get("workspace_id");
  const workspaceId = enabled && raw && /^\d+$/.test(raw) ? Number(raw) : null;

  const setWorkspaceId = useCallback(
    (id) => {
      const search = new URLSearchParams(location.search);
      if (id) search.set("workspace_id", String(id));
      else search.delete("workspace_id");
      const qs = search.toString();
      navigate({ pathname: location.pathname, search: qs ? `?${qs}` : "" }, { replace: true });
    },
    [location.pathname, location.search, navigate],
  );

  const params = useMemo(() => (workspaceId ? { workspace_id: workspaceId } : {}), [workspaceId]);
  return { workspaceId, setWorkspaceId, params };
};

/** WorkspaceFilter is the "All Workspaces" select above a list. */
export const WorkspaceFilter = ({ value, onChange, sx }) => {
  const { enabled, workspaces } = useWorkspaces();
  if (!enabled) return null;
  return (
    <FormControl size="small" sx={{ minWidth: 220, ...sx }}>
      <InputLabel id="workspace-filter-label">Workspace</InputLabel>
      <Select
        labelId="workspace-filter-label"
        label="Workspace"
        data-testid="workspace-filter"
        value={value || ""}
        onChange={(e) => onChange(e.target.value ? Number(e.target.value) : null)}
      >
        <MenuItem value="">All Workspaces</MenuItem>
        {workspaces.map((w) => (
          <MenuItem key={w.id} value={w.id}>
            {w.name}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
};

/**
 * useWorkspaceAccess returns { canOn(workspaceId, perm), canAnywhere(perm) }
 * for gating per-object controls (edit, delete, publish) and create buttons.
 * It falls back to the global check where usePermissions is mocked.
 */
export const useWorkspaceAccess = () => {
  const { can, canIn, canAnywhere } = usePermissions();
  return useMemo(
    () => ({
      canOn: (workspaceId, perm) => (canIn ? canIn(workspaceId, perm) : can(perm)),
      canAnywhere: (perm) => (canAnywhere ? canAnywhere(perm) : can(perm)),
    }),
    [can, canIn, canAnywhere],
  );
};

/**
 * useCanMove reports whether an edit form offers the move control: the
 * caller holds the write permission in more than one Workspace (a global
 * holder always can).
 */
export const useCanMove = (permission) => {
  const { rbacEnabled, can, workspaces } = usePermissions();
  if (!rbacEnabled) return false;
  if (can(permission)) return true;
  return (workspaces || []).filter((w) => hasPermission(w.permissions, permission)).length > 1;
};

const WORKSPACE_CODES = {
  workspace_required: "You can create this in several Workspaces; choose one.",
  invalid_workspace: "This Workspace does not exist or you cannot create objects in it.",
};

/**
 * workspaceFieldError returns the message to show on the Workspace field for
 * an API error (422 workspace_required or invalid_workspace), or "" when the
 * error is about something else.
 */
export const workspaceFieldError = (error) => {
  const errs = error?.response?.data?.errors || error?.data?.errors || error?.errors;
  if (!Array.isArray(errs)) return "";
  const hit = errs.find((e) => e && WORKSPACE_CODES[e.code]);
  return hit ? WORKSPACE_CODES[hit.code] : "";
};
