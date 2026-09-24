import React, { useCallback, useEffect, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import { Box, Button, FormControl, FormHelperText, InputLabel, Link, MenuItem, Select, Typography } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import apiClient from "../../utils/apiClient";
import { listAll } from "../../utils/listAll";
import { usePermissions } from "../../context/PermissionsContext";
import { P } from "../../rbac/permissions";
import EmbedderCreateDialog from "./EmbedderCreateDialog";
import { describeEmbedder } from "./embedderModel";

/**
 * Pick the embedder a form uses, or create one inline. Reusable by any form
 * that embeds (data sources, semantic routers).
 *
 * - `value`: the embedder id ("" for none), `onChange(id, row)`.
 * - `currentName`: the saved embedder's name, shown read-only when the user
 *   may not list embedders (no embedders:read).
 * - `minPrivacy`: the privacy level the embedder must cover; embedders below
 *   it are listed but flagged, and the create dialog starts from it.
 * - `allowNone`: offer "None" (a router without an embedding stage).
 */
const EmbedderPicker = ({
  id = "embedder-picker",
  value,
  onChange,
  error,
  helperText,
  currentName = "",
  minPrivacy = 0,
  allowNone = false,
  required = false,
  label = "Embedder",
}) => {
  const { can } = usePermissions();
  const canRead = can(P.EMBEDDERS_READ);
  const canCreate = can(P.EMBEDDERS_WRITE);
  const [rows, setRows] = useState([]);
  const [loadFailed, setLoadFailed] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);

  const load = useCallback(async () => {
    if (!canRead) return;
    try {
      const response = await listAll(apiClient, "/embedders");
      setRows(response.data.data || []);
      setLoadFailed(false);
    } catch (e) {
      setLoadFailed(true);
    }
  }, [canRead]);

  useEffect(() => {
    load();
  }, [load]);

  const selected = rows.find((row) => String(row.id) === String(value));
  const tooLow = selected && (selected.attributes?.privacy_score ?? 0) < minPrivacy;

  if (!canRead || loadFailed) {
    return (
      <Box data-testid={`${id}-readonly`}>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <Typography variant="body1">{currentName || (value ? `Embedder #${value}` : "None")}</Typography>
        <FormHelperText>Your role does not include access to embedders, so this cannot be changed here.</FormHelperText>
      </Box>
    );
  }

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "flex-start" }}>
        <FormControl fullWidth required={required} error={Boolean(error) || Boolean(tooLow)}>
          <InputLabel id={`${id}-label`}>{label}</InputLabel>
          <Select
            labelId={`${id}-label`}
            label={label}
            value={value ? String(value) : ""}
            onChange={(e) => {
              const row = rows.find((r) => String(r.id) === e.target.value);
              onChange(e.target.value, row || null);
            }}
            inputProps={{ "data-testid": `${id}-select` }}
          >
            {allowNone && (
              <MenuItem value="">
                <em>None</em>
              </MenuItem>
            )}
            {rows.map((row) => (
              <MenuItem key={row.id} value={String(row.id)}>
                <Box>
                  <Typography variant="body2">{row.attributes?.name}</Typography>
                  <Typography variant="caption" color="text.secondary">
                    {describeEmbedder(row.attributes)} · privacy {row.attributes?.privacy_score ?? 0}
                  </Typography>
                </Box>
              </MenuItem>
            ))}
          </Select>
          <FormHelperText>
            {error ||
              (tooLow
                ? `This embedder's privacy level (${selected.attributes.privacy_score}) is below ${minPrivacy}; saving will be refused.`
                : helperText)}
          </FormHelperText>
        </FormControl>
        {canCreate && (
          <Button
            variant="outlined"
            startIcon={<AddIcon />}
            onClick={() => setDialogOpen(true)}
            sx={{ whiteSpace: "nowrap", mt: 1 }}
          >
            New embedder
          </Button>
        )}
      </Box>
      {selected && (
        <Link component={RouterLink} to={`/admin/embedders/${selected.id}`} variant="caption">
          View {selected.attributes?.name}
        </Link>
      )}
      <EmbedderCreateDialog
        open={dialogOpen}
        minPrivacy={minPrivacy}
        onClose={() => setDialogOpen(false)}
        onCreated={(row) => {
          setDialogOpen(false);
          setRows((prev) => [...prev, row]);
          onChange(String(row.id), row);
        }}
      />
    </Box>
  );
};

export default EmbedderPicker;
