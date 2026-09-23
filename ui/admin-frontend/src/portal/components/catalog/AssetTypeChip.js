import React from "react";
import { Chip, useTheme } from "@mui/material";
import { CATALOG_TYPES, itemTypeLabel } from "../../utils/catalog";

/**
 * One small, colour-coded label per asset type so a mixed grid of cards can
 * be scanned by type. Colours come from the theme's surface tokens so they
 * follow branding.
 */
const AssetTypeChip = ({ item, type, label, size = "small", sx = {} }) => {
  const theme = useTheme();
  const resolvedType = type || item?.type;
  const surfaces = {
    [CATALOG_TYPES.LLM]: theme.palette.background.surfaceBrandDefaultDashboard,
    // A model router fronts LLM providers, so it shares their colour.
    [CATALOG_TYPES.MODEL_ROUTER]: theme.palette.background.surfaceBrandDefaultDashboard,
    [CATALOG_TYPES.DATASOURCE]: theme.palette.background.surfaceInformativeDefault,
    [CATALOG_TYPES.TOOL]: theme.palette.background.surfaceSuccessDefault,
    [CATALOG_TYPES.PLUGIN_RESOURCE]: theme.palette.background.surfaceWarningDefault,
  };
  return (
    <Chip
      size={size}
      label={label || itemTypeLabel(item || { type: resolvedType })}
      data-testid="asset-type-chip"
      data-asset-type={resolvedType}
      sx={{
        backgroundColor: surfaces[resolvedType] || theme.palette.background.neutralDefault,
        color: theme.palette.text.primary,
        fontFamily: "Inter-Medium",
        fontSize: "0.72rem",
        height: 22,
        borderRadius: "6px",
        ...sx,
      }}
    />
  );
};

export default AssetTypeChip;
