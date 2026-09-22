import React, { useState, useEffect } from "react";
import { Box, Typography, CircularProgress } from "@mui/material";
import apiClient from "../../../utils/apiClient";
import RelationshipPicker from "../../common/relationship-picker";

/**
 * GroupPluginResourcesSection renders a RelationshipPicker for each registered
 * plugin resource type, allowing admins to assign plugin resource instances
 * to a group for access control.
 *
 * This replaces the Catalogue pattern for plugin resources — instances map
 * directly to groups without an intermediate catalogue layer.
 *
 * The selection is reported through `onChange` as
 * `{ "<pluginId>:<slug>": [instanceId] }` and saved by the team form
 * (PUT /groups/:id/plugin-resources). It is only reported once the resource
 * types have loaded, so a save never clears assignments the user never saw.
 */
export const APP_GRANTED_HELPER_TEXT =
  "Members will only be able to select these resources when creating apps.";
export const PLUGIN_GRANTED_HELPER_TEXT =
  "Members can see these resources in the portal. They are not selectable when creating apps; access is granted by the plugin itself.";

/**
 * What a team assignment means for one resource type. Types that grant
 * access through apps (the default, and the legacy rows without the flag)
 * become pickable in the app form; the others are visible in the portal and
 * the plugin decides access on its own.
 */
export const resourceTypeHelperText = (rt) =>
  rt?.access_granted_via_app === false ? PLUGIN_GRANTED_HELPER_TEXT : APP_GRANTED_HELPER_TEXT;

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

  // Notify parent of changes -- but only once we know what the section
  // shows. Reporting `{}` before the types load would make the form save an
  // empty assignment list.
  useEffect(() => {
    if (loading || resourceTypes.length === 0) return;
    onChange?.(selections);
  }, [selections, onChange, loading, resourceTypes.length]);

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
        Assign plugin resource instances to this team.
      </Typography>

      {resourceTypes.map((rt) => {
        const key = `${rt.plugin_id}:${rt.slug}`;
        const typeInstances = instances[key] || [];
        const selectedIds = selections[key] || [];
        // Selected ids are resolved against the loaded instances; an id whose
        // instance did not load is still shown (by id) rather than dropped.
        const selectedInstances = selectedIds.map(
          (val) => typeInstances.find((i) => i.id === val) || { id: val, name: val },
        );

        return (
          <Box key={key} sx={{ mb: 3 }}>
            <RelationshipPicker
              label={rt.name}
              itemLabel={rt.name}
              helperText={resourceTypeHelperText(rt)}
              value={selectedInstances}
              options={typeInstances}
              onChange={(items) => {
                setSelections((prev) => ({
                  ...prev,
                  [key]: items.map((i) => i.id),
                }));
              }}
            />
          </Box>
        );
      })}
    </Box>
  );
};

export default GroupPluginResourcesSection;
