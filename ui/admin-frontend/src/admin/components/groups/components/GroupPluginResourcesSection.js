import React, { useState, useEffect } from "react";
import {
  Box,
  Typography,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  CircularProgress,
} from "@mui/material";
import apiClient from "../../../utils/apiClient";

/**
 * GroupPluginResourcesSection renders a multi-select for each registered
 * plugin resource type, allowing admins to assign plugin resource instances
 * to a group for access control.
 *
 * This replaces the Catalogue pattern for plugin resources — instances map
 * directly to groups without an intermediate catalogue layer.
 */
const GroupPluginResourcesSection = ({ groupId, onChange }) => {
  const [resourceTypes, setResourceTypes] = useState([]);
  const [instances, setInstances] = useState({}); // { "pluginId:slug": [...] }
  const [selections, setSelections] = useState({}); // { "pluginId:slug": [...ids] }
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const load = async () => {
      try {
        // Fetch registered resource types
        const typesResp = await apiClient.get("/plugin-resource-types");
        const types = typesResp.data.data || [];
        setResourceTypes(types);

        // Fetch current group assignments if editing
        if (groupId) {
          try {
            const groupResp = await apiClient.get(
              `/groups/${groupId}/plugin-resources`,
            );
            const groupData = groupResp.data.data || [];
            const sel = {};
            for (const pr of groupData) {
              sel[`${pr.plugin_id}:${pr.resource_type_slug}`] =
                pr.instance_ids || [];
            }
            setSelections(sel);
          } catch {
            // Group may not have any plugin resources yet
          }
        }

        // Fetch instances for each type
        for (const rt of types) {
          try {
            const instResp = await apiClient.get(
              `/plugin-resource-types/${rt.plugin_id}/${rt.slug}/instances`,
            );
            if (instResp.data && instResp.data.data) {
              setInstances((prev) => ({
                ...prev,
                [`${rt.plugin_id}:${rt.slug}`]: instResp.data.data,
              }));
            }
          } catch {
            // Instance endpoint may not be wired yet
          }
        }
      } catch {
        // Plugin resource types not available
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [groupId]);

  // Notify parent of changes
  useEffect(() => {
    if (onChange) {
      onChange(selections);
    }
  }, [selections, onChange]);

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  if (resourceTypes.length === 0) {
    return null; // No plugin resource types registered — hide section entirely
  }

  return (
    <Box sx={{ mt: 3 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Plugin Resources
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Assign plugin resource instances to this group. Members will only be
        able to select these resources when creating apps.
      </Typography>

      {resourceTypes.map((rt) => {
        const key = `${rt.plugin_id}:${rt.slug}`;
        const typeInstances = instances[key] || [];
        const selected = selections[key] || [];

        return (
          <FormControl fullWidth key={key} sx={{ mb: 2 }}>
            <InputLabel>{rt.name}</InputLabel>
            <Select
              multiple
              value={selected}
              onChange={(e) => {
                setSelections((prev) => ({
                  ...prev,
                  [key]: e.target.value,
                }));
              }}
              renderValue={(sel) => (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                  {sel.map((val) => {
                    const inst = typeInstances.find((i) => i.id === val);
                    return (
                      <Chip key={val} label={inst ? inst.name : val} />
                    );
                  })}
                </Box>
              )}
            >
              {typeInstances.map((inst) => (
                <MenuItem key={inst.id} value={inst.id}>
                  {inst.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        );
      })}
    </Box>
  );
};

export default GroupPluginResourcesSection;
