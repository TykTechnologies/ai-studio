import React from "react";
import { Typography, Box } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import CustomSelectMany from "../../common/CustomSelectMany";
import CustomNote from "../../common/CustomNote";

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
  
  loading
}) => {
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
        <Box sx={{ mb: 2 }}>
          <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
            LLM providers catalogs
          </Typography>
          <CustomSelectMany
            value={selectedCatalogs}
            onChange={onCatalogsChange}
            options={catalogs}
            disabled={loading}
          />
        </Box>
        
        <Box sx={{ mb: 2 }}>
          <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
            Data sources catalogs
          </Typography>
          <CustomSelectMany
            value={selectedDataCatalogs}
            onChange={onDataCatalogsChange}
            options={dataCatalogs}
            disabled={loading}
          />
        </Box>
        
        <Box sx={{ mb: 2 }}>
          <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
            Tools catalogs
          </Typography>
          <CustomSelectMany
            value={selectedToolCatalogs}
            onChange={onToolCatalogsChange}
            options={toolCatalogs}
            disabled={loading}
          />
        </Box>
        </>
      )}
    </CollapsibleSection>
  );
};

export default GroupCatalogsSection;