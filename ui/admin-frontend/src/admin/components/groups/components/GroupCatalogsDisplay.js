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
import OtherWorkspacesNote from "../../catalogues/OtherWorkspacesNote";
import { usePermissions } from "../../../context/PermissionsContext";
import { P } from "../../../rbac/permissions";

// Workspaces (Enterprise): the team response lists only the catalogs the
// caller can see. A caller without a global read of a kind is told that
// others may exist, so an empty or short list does not read as the team's.
const READ_PERMISSION = {
  llm: P.CATALOGUES_READ,
  data: P.DATA_CATALOGUES_READ,
  tool: P.TOOL_CATALOGUES_READ,
};

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
  const { rbacEnabled, can } = usePermissions();
  const mayHideSome =
    Boolean(rbacEnabled) &&
    visibleCatalogTypes.some((type) => READ_PERMISSION[type.variant] && !can(READ_PERMISSION[type.variant]));

  return (
    <CollapsibleSection title={title} defaultExpanded={defaultExpanded}>
      {hasNoCatalogs ? (
        <CustomNote
          message={
            mayHideSome
              ? "No catalogs in your Workspaces are assigned to this team. Catalogs in other Workspaces are not listed."
              : emptyMessage
          }
        />
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
                  {READ_PERMISSION[catalogType.variant] && (
                    <OtherWorkspacesNote
                      permission={READ_PERMISSION[catalogType.variant]}
                      sx={{ gridColumn: 2, ml: 6, mt: -1.5 }}
                    >
                      Catalogs in Workspaces you cannot see are not listed.
                    </OtherWorkspacesNote>
                  )}
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