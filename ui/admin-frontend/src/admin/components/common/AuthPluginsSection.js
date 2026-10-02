import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import CloseIcon from "@mui/icons-material/Close";
import apiClient from "../../utils/apiClient";
import Section from "./Section";
import { usePermissions } from "../../context/PermissionsContext";
import { PrimaryButton } from "../../styles/sharedStyles";

// The /plugins list endpoint pages with `limit` (capped at 100).
const PLUGIN_PAGE_SIZE = 100;
const MAX_PLUGIN_PAGES = 10;

const supportsAuth = (attrs) =>
  attrs?.hook_type === "auth" || (Array.isArray(attrs?.hook_types) && attrs.hook_types.includes("auth"));

const toPlugin = (row) => ({
  id: String(row.id),
  name: row.attributes?.name || `Plugin ${row.id}`,
  description: row.attributes?.description || "",
  namespace: row.attributes?.namespace || "",
});

/** Every active plugin that provides the auth hook, primary or not. */
export const fetchAuthCapablePlugins = async () => {
  const found = [];
  for (let page = 1; page <= MAX_PLUGIN_PAGES; page += 1) {
    const response = await apiClient.get("/plugins", {
      params: { page, limit: PLUGIN_PAGE_SIZE, is_active: true },
    });
    const rows = response.data?.data || [];
    rows.filter((row) => supportsAuth(row.attributes)).forEach((row) => found.push(toPlugin(row)));
    if (rows.length < PLUGIN_PAGE_SIZE) break;
  }
  return found;
};

/**
 * The auth plugins of one gateway endpoint (a datasource, tool, router or
 * custom-endpoint plugin): when any are attached they alone authenticate the
 * endpoint's requests on the gateway, tried in order, and app keys are
 * refused. The list saves on its own, apart from the endpoint's form.
 *
 * endpointPath is the REST collection ("datasources", "tools",
 * "model-routers", "semantic-routers", "plugins"); noun names the endpoint
 * in the help text; writePermission (an rbac/permissions P entry) decides
 * whether the viewer may change the list.
 */
const AuthPluginsSection = ({ endpointPath, endpointId, noun = "endpoint", writePermission }) => {
  const { can } = usePermissions();
  const canEdit = writePermission ? can(writePermission) : false;
  const [available, setAvailable] = useState([]);
  const [selected, setSelected] = useState([]);
  const [saved, setSaved] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const listUrl = `/${endpointPath}/${endpointId}/auth-plugins`;

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [plugins, current] = await Promise.all([fetchAuthCapablePlugins(), apiClient.get(listUrl)]);
      const currentPlugins = (current.data?.data || []).map(toPlugin);
      // An attached plugin that is no longer offered (inactive) still shows.
      const byId = new Map(plugins.map((p) => [p.id, p]));
      currentPlugins.forEach((p) => {
        if (!byId.has(p.id)) byId.set(p.id, p);
      });
      setAvailable(Array.from(byId.values()));
      const ids = currentPlugins.map((p) => p.id);
      setSelected(ids);
      setSaved(ids);
    } catch (e) {
      setError(e.response?.data?.errors?.[0]?.detail || e.message || "Failed to load auth plugins");
    } finally {
      setLoading(false);
    }
  }, [listUrl]);

  useEffect(() => {
    if (endpointId) load();
  }, [endpointId, load]);

  const byId = useMemo(() => new Map(available.map((p) => [p.id, p])), [available]);
  const dirty = selected.join(",") !== saved.join(",");

  const move = (index, delta) => {
    const next = [...selected];
    const [item] = next.splice(index, 1);
    next.splice(index + delta, 0, item);
    setSelected(next);
  };

  const save = async () => {
    setSaving(true);
    setError("");
    setMessage("");
    try {
      await apiClient.put(listUrl, { plugin_ids: selected.map((id) => parseInt(id, 10)) });
      setSaved(selected);
      setMessage("Auth plugins saved. Edges apply them after the next configuration push.");
    } catch (e) {
      setError(e.response?.data?.errors?.[0]?.detail || e.message || "Failed to save auth plugins");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Section
      title="Authentication plugins"
      description={`When any are attached, only these plugins authenticate gateway requests to this ${noun}, tried in order; app keys are refused. With none, app keys apply as usual.`}
      data-testid="auth-plugins-section"
    >
      {loading ? (
        <Box display="flex" justifyContent="center" py={2}>
          <CircularProgress size={24} />
        </Box>
      ) : (
        <Stack spacing={2}>
          {error && <Alert severity="error">{error}</Alert>}
          {message && <Alert severity="success">{message}</Alert>}

          {selected.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No auth plugins: this {noun} authenticates with app keys.
            </Typography>
          ) : (
            <Stack spacing={1}>
              {selected.map((id, index) => {
                const plugin = byId.get(id);
                return (
                  <Box key={id} display="flex" alignItems="center" gap={1}>
                    <Typography variant="body2" sx={{ minWidth: 24 }}>
                      {index + 1}.
                    </Typography>
                    <Chip label={plugin ? plugin.name : `Plugin ${id}`} size="small" color="primary" variant="outlined" />
                    {plugin?.namespace && (
                      <Typography variant="caption" color="text.secondary">
                        {plugin.namespace}
                      </Typography>
                    )}
                    <Box flex={1} />
                    {canEdit && (
                      <>
                        <Tooltip title="Earlier">
                          <span>
                            <IconButton size="small" aria-label={`move ${plugin?.name || id} up`} disabled={index === 0} onClick={() => move(index, -1)}>
                              <ArrowUpwardIcon fontSize="small" />
                            </IconButton>
                          </span>
                        </Tooltip>
                        <Tooltip title="Later">
                          <span>
                            <IconButton size="small" aria-label={`move ${plugin?.name || id} down`} disabled={index === selected.length - 1} onClick={() => move(index, 1)}>
                              <ArrowDownwardIcon fontSize="small" />
                            </IconButton>
                          </span>
                        </Tooltip>
                        <Tooltip title="Detach">
                          <IconButton size="small" aria-label={`detach ${plugin?.name || id}`} onClick={() => setSelected(selected.filter((s) => s !== id))}>
                            <CloseIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </>
                    )}
                  </Box>
                );
              })}
            </Stack>
          )}

          {canEdit && (
            <Box display="flex" alignItems="center" gap={2}>
              <FormControl size="small" sx={{ minWidth: 280 }}>
                <InputLabel id={`auth-plugins-add-${endpointPath}`}>Attach an auth plugin</InputLabel>
                <Select
                  labelId={`auth-plugins-add-${endpointPath}`}
                  label="Attach an auth plugin"
                  value=""
                  onChange={(e) => e.target.value && setSelected([...selected, e.target.value])}
                >
                  {available.filter((p) => !selected.includes(p.id)).length === 0 && (
                    <MenuItem value="" disabled>
                      No other plugins provide the auth hook
                    </MenuItem>
                  )}
                  {available
                    .filter((p) => !selected.includes(p.id))
                    .map((p) => (
                      <MenuItem key={p.id} value={p.id}>
                        <Box>
                          <Typography variant="body2">{p.name}</Typography>
                          {p.description && (
                            <Typography variant="caption" color="text.secondary">
                              {p.description}
                            </Typography>
                          )}
                        </Box>
                      </MenuItem>
                    ))}
                </Select>
              </FormControl>
              <PrimaryButton variant="contained" disabled={!dirty || saving} onClick={save}>
                {saving ? "Saving…" : "Save auth plugins"}
              </PrimaryButton>
            </Box>
          )}
        </Stack>
      )}
    </Section>
  );
};

export default AuthPluginsSection;
