import React from "react";
import { Typography, Box } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import CustomSelectMany from "../../common/CustomSelectMany";

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
  return (
    <CollapsibleSection title="Add catalogs" defaultExpanded={false}>
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
    </CollapsibleSection>
  );
};

export default GroupCatalogsSection;