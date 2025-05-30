import React from "react";
import { Typography, Box } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import CollapsibleSection from "../../common/CollapsibleSection";
import CustomNote from "../../common/CustomNote";
import { getFeatureFlags } from "../../../utils/featureUtils";
import { StyledChip } from "../../../styles/sharedStyles";
import { CatalogContainer, getColorsForVariant } from "./styles";
import { CATALOG_DISPLAY_STYLES } from "../utils/config";

const GroupCatalogsDisplay = ({
  catalogues = [],
  dataCatalogues = [],
  toolCatalogues = [],
  features = {},
  title = "Catalogs",
  defaultExpanded = true,
  emptyMessage = "No catalogs are currently assigned to this team."
}) => {
  const theme = useTheme();
  const { isPortalOnly, isChatOnly } = getFeatureFlags(features);
  
  const catalogTypes = [
    {
      label: "LLM providers",
      items: catalogues,
      show: isPortalOnly || !isChatOnly,
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
      show: isChatOnly || !isPortalOnly,
      variant: "tool"
    }
  ];

  const visibleCatalogTypes = catalogTypes.filter(type => type.show);
  const hasNoCatalogs = visibleCatalogTypes.every(type => !type.items || type.items.length === 0);

  return (
    <CollapsibleSection title={title} defaultExpanded={defaultExpanded}>
      {hasNoCatalogs ? (
        <CustomNote
          message={emptyMessage}
        />
      ) : (
        <>
          {visibleCatalogTypes.map((catalogType, idx) => {
            const { bgColor, textColor } = getColorsForVariant(theme, catalogType.variant);
            
            return (
              <Box
                key={catalogType.label}
                sx={idx < visibleCatalogTypes.length - 1 ? CATALOG_DISPLAY_STYLES.borderStyle : CATALOG_DISPLAY_STYLES.lastRowStyle}
              >
                <Typography
                  variant="bodyLargeBold"
                  color="text.primary"
                  sx={{ minWidth: 140 }}
                >
                  {catalogType.label}
                </Typography>
                <CatalogContainer>
                  {catalogType.items && catalogType.items.length > 0 ? (
                    catalogType.items.map((item) => (
                      <StyledChip
                        key={item.id}
                        label={item.attributes?.name || item.name || `Item ${item.id}`}
                        size="small"
                        bgColor={bgColor}
                        textColor={textColor}
                      />
                    ))
                  ) : (
                    <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                      None
                    </Typography>
                  )}
                </CatalogContainer>
              </Box>
            );
          })}
        </>
      )}
    </CollapsibleSection>
  );
};

export default GroupCatalogsDisplay; 