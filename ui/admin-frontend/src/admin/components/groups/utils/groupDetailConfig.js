import { Box, Typography } from "@mui/material";
import CustomSelectBadge from "../../common/CustomSelectBadge";
import { roleBadgeConfigs } from "./roleBadgeConfig";
import { getFeatureFlags } from "../../../utils/featureUtils";

export const CATALOG_ROWS = [
    {
      label: "LLM providers",
      itemsKey: "catalogues",
    },
    {
      label: "Data sources",
      itemsKey: "dataCatalogues",
    },
    {
      label: "Tools",
      itemsKey: "toolCatalogues",
    },
  ];

export const borderStyle = {
  borderBottom: "2px solid",
  borderColor: "border.neutralDefaultSubdued",
  padding: "16px 0",
  display: "flex",
  alignItems: "center",
};

export const lastRowStyle = {
  padding: "16px 0",
  display: "flex",
  alignItems: "center",
};

export const TEAM_MEMBERS_COLUMNS = [
  { field: "name", headerName: "Name", width: "30%" },
  { field: "email", headerName: "Email", width: "40%" },
  { field: "role", headerName: "Role", width: "30%" },
];

export const TEAM_MEMBERS_COLUMNS_FOR_TABLE = [
  {
    field: "name",
    headerName: "Name",
    width: { md: '35%', lg: '40%' },
    renderCell: (row) => (
      (
        <Box sx={{
          display: 'flex',
          flexDirection: 'column',
          width: '100%',
          pr: 1
        }}>
          <Typography
            variant="bodyMediumMedium"
            color="text.defaultSubdued"
            sx={{
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              width: '100%'
            }}
          >
            {row.name}
          </Typography>
          <Typography
            variant="bodySmallDefault"
            color="text.defaultSubdued"
            sx={{
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              width: '100%'
            }}
          >
            {row.email}
          </Typography>
        </Box>
      )
    )
  },
  {
    field: "role",
    headerName: "Role",
    width: { md: '45%', lg: '35%' },
    renderCell: (row) => (
      <CustomSelectBadge config={roleBadgeConfigs[row.role] || roleBadgeConfigs["Chat user"]} />
    )
  }
];

export const CATALOG_DISPLAY_STYLES = {
  borderStyle: {
    borderBottom: "2px solid",
    borderColor: "border.neutralDefaultSubdued",
    padding: "16px 0",
    display: "flex",
    alignItems: "center",
  },
  
  lastRowStyle: {
    padding: "16px 0",
    display: "flex",
    alignItems: "center",
  }
};

export const getCatalogTypes = (features, catalogues, dataCatalogues, toolCatalogues) => {
  const { isPortalEnabled, isChatEnabled } = getFeatureFlags(features);
  
  return [
    {
      label: "LLM providers",
      items: catalogues,
      show: isPortalEnabled,
      variant: "llm"
    },
    {
      label: "Data sources", 
      items: dataCatalogues,
      show: true,
      variant: "data"
    },
    {
      label: "Tools",
      items: toolCatalogues,
      show: isChatEnabled,
      variant: "tool"
    }
  ];
};

export const GROUP_CATALOGS_DEFAULTS = {
  title: "Catalogs",
  defaultExpanded: true,
  emptyMessage: "No catalogs are currently assigned to this team."
}; 