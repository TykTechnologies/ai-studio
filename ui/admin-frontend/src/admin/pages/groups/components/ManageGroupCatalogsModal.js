import React, { useState } from "react";
import { Box, Typography, CircularProgress } from "@mui/material";
import ActionModal from "../../../components/common/ActionModal";
import CustomSelectMany from "../../../components/common/CustomSelectMany";
import { useCatalogsModal } from "../hooks/useCatalogsModal";
import { teamsService } from "../../../services/teamsService";
import { calculateGroupCatalogPayload } from "../../../services/utils/teamsServiceUtils";
import { getFeatureFlags } from "../../../utils/featureUtils";

const ManageGroupCatalogsModal = ({ 
  open, 
  onClose, 
  group, 
  onSuccess,
  onError,
  features
}) => {
  const [saving, setSaving] = useState(false);
  const { isPortalOnly, isChatOnly, isGatewayOnly } = getFeatureFlags(features);
  
  const {
    catalogs,
    selectedCatalogs,
    setSelectedCatalogs,
    dataCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    toolCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    loading
  } = useCatalogsModal(group?.id, features);

  if (isGatewayOnly) {
    return null;
  }

  const handleSave = async () => {
    if (!group) return;
    
    setSaving(true);
    try {
      const catalogData = calculateGroupCatalogPayload(
        selectedCatalogs,
        selectedDataCatalogs,
        selectedToolCatalogs
      );
      
      await teamsService.updateGroupCatalogs(group.id, catalogData);
      onSuccess(`Catalogs for "${group.attributes.name}" updated successfully!`);
      onClose();
    } catch (error) {
      onError("Failed to update catalogs. Please try again.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <ActionModal
      open={open}
      title="Manage Catalogs"
      onClose={onClose}
      onPrimaryAction={loading ? () => {} : handleSave}
      onSecondaryAction={onClose}
      disabled={saving || loading}
    >
      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      ) : (
        <>
          <Box sx={{ mb: 3 }}>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              Select one or more catalogs to make available to this team
            </Typography>
          </Box>
          
          {(isPortalOnly || !isChatOnly) && (
            <Box sx={{ mb: 3 }}>
              <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
                LLM providers catalogs
              </Typography>
              <CustomSelectMany
                value={selectedCatalogs}
                onChange={setSelectedCatalogs}
                options={catalogs}
                disabled={saving}
                chipVariant="llm"
              />
            </Box>
          )}
          
          <Box sx={{ mb: 3 }}>
            <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
              Data sources catalogs
            </Typography>
            <CustomSelectMany
              value={selectedDataCatalogs}
              onChange={setSelectedDataCatalogs}
              options={dataCatalogs}
              disabled={saving}
              chipVariant="data"
            />
          </Box>
          
          {(isChatOnly || !isPortalOnly) && (
            <Box sx={{ mb: 2 }}>
              <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
                Tools catalogs
              </Typography>
              <CustomSelectMany
                value={selectedToolCatalogs}
                onChange={setSelectedToolCatalogs}
                options={toolCatalogs}
                disabled={saving}
                chipVariant="tool"
              />
            </Box>
          )}
        </>
      )}
    </ActionModal>
  );
};

export default ManageGroupCatalogsModal;