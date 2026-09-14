import React, { useState } from "react";
import { Box, Typography, CircularProgress } from "@mui/material";
import ActionModal from "../../../components/common/ActionModal";
import RelationshipPicker from "../../../components/common/relationship-picker";
import { useCatalogsModal } from "../hooks/useCatalogsModal";
import { teamsService } from "../../../services/teamsService";
import { calculateGroupCatalogPayload } from "../../../services/utils/teamsServiceUtils";
import { getFeatureFlags } from "../../../utils/featureUtils";

// Catalog options and selections are `{ value, label }` pairs (see
// useCatalogsSelection), hence idField="value" and the label accessor.
const catalogLabel = (catalog) => catalog?.label ?? "";

const ManageGroupCatalogsModal = ({ 
  open, 
  onClose, 
  group, 
  onSuccess,
  onError,
  features
}) => {
  const [saving, setSaving] = useState(false);
  const { isPortalEnabled, isChatEnabled, isGatewayOnly } = getFeatureFlags(features);
  
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
          
          {isPortalEnabled && (
            <Box sx={{ mb: 3 }}>
              <RelationshipPicker
                label="LLM providers catalogs"
                itemLabel="LLM catalog"
                idField="value"
                value={selectedCatalogs}
                onChange={setSelectedCatalogs}
                options={catalogs}
                getOptionLabel={catalogLabel}
                disabled={saving}
              />
            </Box>
          )}
          
          <Box sx={{ mb: 3 }}>
            <RelationshipPicker
              label="Data sources catalogs"
              itemLabel="data catalog"
              idField="value"
              value={selectedDataCatalogs}
              onChange={setSelectedDataCatalogs}
              options={dataCatalogs}
              getOptionLabel={catalogLabel}
              disabled={saving}
            />
          </Box>
          
          {isChatEnabled && (
            <Box sx={{ mb: 2 }}>
              <RelationshipPicker
                label="Tools catalogs"
                itemLabel="tool catalog"
                idField="value"
                value={selectedToolCatalogs}
                onChange={setSelectedToolCatalogs}
                options={toolCatalogs}
                getOptionLabel={catalogLabel}
                disabled={saving}
              />
            </Box>
          )}
        </>
      )}
    </ActionModal>
  );
};

export default ManageGroupCatalogsModal;