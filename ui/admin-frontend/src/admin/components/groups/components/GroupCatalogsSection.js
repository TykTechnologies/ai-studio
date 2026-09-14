import React from "react";
import { Typography, Box } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import RelationshipPicker from "../../common/relationship-picker";
import CustomNote from "../../common/CustomNote";
import { getFeatureFlags } from "../../../utils/featureUtils";

// Catalog options and selections are `{ value, label }` pairs (see
// useCatalogsSelection), hence idField="value" and the label accessor.
const catalogLabel = (catalog) => catalog?.label ?? "";

const GroupCatalogsSection = ({
  catalogs,
  selectedCatalogs,
  onCatalogsChange,
  
  dataCatalogs,
  selectedDataCatalogs,
  onDataCatalogsChange,
  
  toolCatalogs,
  selectedToolCatalogs,
  onToolCatalogsChange,
  
  loading,
  features
}) => {
  const { isPortalEnabled, isChatEnabled } = getFeatureFlags(features);
  const hasNoCatalogs = (!catalogs || catalogs.length === 0) &&
                        (!dataCatalogs || dataCatalogs.length === 0) &&
                        (!toolCatalogs || toolCatalogs.length === 0);

  return (
    <CollapsibleSection title="Add catalogs" defaultExpanded={false}>
      {hasNoCatalogs ? (
        <CustomNote
          message="Currently, there are no catalogs available. To create a new one, please go to the Catalogs."
        />
      ) : (
        <>
        <Box sx={{ mt: -2, mb: 2 }}>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            Select one or more catalogs to make available to this team
          </Typography>
        </Box>
        
        {isPortalEnabled && (
          <Box sx={{ mb: 3 }}>
            <RelationshipPicker
              label="LLM providers catalogs"
              itemLabel="LLM catalog"
              idField="value"
              value={selectedCatalogs}
              onChange={onCatalogsChange}
              options={catalogs}
              getOptionLabel={catalogLabel}
              disabled={loading}
            />
          </Box>
        )}
        
        <Box sx={{ mb: 3 }}>
          <RelationshipPicker
            label="Data sources catalogs"
            itemLabel="data catalog"
            idField="value"
            value={selectedDataCatalogs}
            onChange={onDataCatalogsChange}
            options={dataCatalogs}
            getOptionLabel={catalogLabel}
            disabled={loading}
          />
        </Box>
        
        {isChatEnabled && (
          <Box sx={{ mb: 2 }}>
            <RelationshipPicker
              label="Tools catalogs"
              itemLabel="tool catalog"
              idField="value"
              value={selectedToolCatalogs}
              onChange={onToolCatalogsChange}
              options={toolCatalogs}
              getOptionLabel={catalogLabel}
              disabled={loading}
            />
          </Box>
        )}
        </>
      )}
    </CollapsibleSection>
  );
};

export default GroupCatalogsSection;