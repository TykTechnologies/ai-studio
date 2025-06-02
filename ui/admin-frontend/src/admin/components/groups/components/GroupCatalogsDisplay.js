import React from "react";
import { Typography } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import CollapsibleSection from "../../common/CollapsibleSection";
import CustomNote from "../../common/CustomNote";
import { StyledChip } from "../../../styles/sharedStyles";
import { 
  CatalogContainer, 
  getColorsForVariant,
  CatalogTypeContainer,
  CatalogTypeLabel,
  CatalogsWrapper,
  CatalogBorderLine,
} from "./styles";
import { 
  getCatalogTypes, 
  GROUP_CATALOGS_DEFAULTS 
} from "../utils/groupDetailConfig";

const GroupCatalogsDisplay = ({
  catalogues = [],
  dataCatalogues = [],
  toolCatalogues = [],
  features = {},
  title = GROUP_CATALOGS_DEFAULTS.title,
  defaultExpanded = GROUP_CATALOGS_DEFAULTS.defaultExpanded,
  emptyMessage = GROUP_CATALOGS_DEFAULTS.emptyMessage
}) => {
  const theme = useTheme();
  const catalogTypes = getCatalogTypes(features, catalogues, dataCatalogues, toolCatalogues);
  const visibleCatalogTypes = catalogTypes.filter(type => type.show);
  const hasNoCatalogs = visibleCatalogTypes.every(type => !type.items || type.items.length === 0);

  return (
    <CollapsibleSection title={title} defaultExpanded={defaultExpanded}>
      {hasNoCatalogs ? (
        <CustomNote message={emptyMessage} />
      ) : (
        <CatalogsWrapper>
          {visibleCatalogTypes.map((catalogType, idx) => {
            const { bgColor, textColor } = getColorsForVariant(theme, catalogType.variant);
            const isLast = idx === visibleCatalogTypes.length - 1;
            
            return (
              <React.Fragment key={catalogType.label}>
                <CatalogTypeContainer>
                  <CatalogTypeLabel
                    variant="bodyLargeBold"
                    color="text.primary"
                  >
                    {catalogType.label}
                  </CatalogTypeLabel>
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
                </CatalogTypeContainer>
                {!isLast && <CatalogBorderLine />}
              </React.Fragment>
            );
          })}
        </CatalogsWrapper>
      )}
    </CollapsibleSection>
  );
};

export default GroupCatalogsDisplay; 