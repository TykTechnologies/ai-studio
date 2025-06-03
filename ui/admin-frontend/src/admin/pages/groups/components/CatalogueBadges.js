import React from "react";
import { Box, Typography, Tooltip } from "@mui/material";
import CustomSelectBadge from "../../../components/common/CustomSelectBadge";
import catalogueBadgeConfigs from "../utils/catalogueBadgeConfig";

const CatalogueBadges = ({ catalogues, dataCatalogues, toolCatalogues }) => {
  const allCatalogues = [
    ...catalogues?.map(name => ({ type: 'llm', name })),
    ...dataCatalogues?.map(name => ({ type: 'data', name })),
    ...toolCatalogues?.map(name => ({ type: 'tool', name }))
  ];

  const totalCount = allCatalogues.length;
  const MAX_BADGES = 3;
  const visibleCatalogues = allCatalogues.slice(0, MAX_BADGES);
  const hasMore = totalCount > MAX_BADGES;

  return (
    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
      {visibleCatalogues.map((cat, index) => (
        <CustomSelectBadge
          key={`${cat.type}-${cat.name}-${index}`}
          config={{
            ...catalogueBadgeConfigs[cat.type],
            text: cat.name
          }}
        />
      ))}

      {hasMore && (
        <Box>
          <Tooltip title={`${totalCount - MAX_BADGES} more catalogues`}>
            <Typography variant="bodySmallDefault" sx={{ ml: 0.5 }}>
              +{totalCount - MAX_BADGES}
            </Typography>
          </Tooltip>
        </Box>
      )}
    </Box>
  );
};

export default CatalogueBadges;