import React, { useState } from "react";
import { Box, Typography, CircularProgress } from "@mui/material";
import ActionModal from "../../../components/common/ActionModal";
import CustomSelectMany from "../../../components/common/CustomSelectMany";
import { useCatalogsModal } from "../hooks/useCatalogsModal";
import { teamsService } from "../../../services/teamsService";

const ManageGroupCatalogsModal = ({ 
  open, 
  onClose, 
  group, 
  onSuccess,
  onError 
}) => {
  const [saving, setSaving] = useState(false);
  
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
  } = useCatalogsModal(group?.id);

  const handleSave = async () => {
    if (!group) return;
    
    setSaving(true);
    try {
      const catalogData = {
        data: {
          type: "Group",
          attributes: {
            catalogues: selectedCatalogs.map(cat => parseInt(cat.value, 10)),
            data_catalogues: selectedDataCatalogs.map(cat => parseInt(cat.value, 10)),
            tool_catalogues: selectedToolCatalogs.map(cat => parseInt(cat.value, 10))
          }
        }
      };
      
      await teamsService.updateGroupCatalogs(group.id, catalogData);
      onSuccess(`Catalogs for "${group.attributes.name}" updated successfully!`);
      onClose();
    } catch (error) {
      console.error("Error updating group catalogs:", error);
      onError("Failed to update catalogs. Please try again.");
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <ActionModal
        open={open}
        title="Manage Catalogs"
        onClose={onClose}
        onPrimaryAction={() => {}}
        onSecondaryAction={onClose}
        disabled={true}
      >
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      </ActionModal>
    );
  }

  return (
    <ActionModal
      open={open}
      title="Manage Catalogs"
      onClose={onClose}
      onPrimaryAction={handleSave}
      onSecondaryAction={onClose}
      disabled={saving}
    >
      <Box sx={{ mb: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Select one or more catalogs to make available to this team
        </Typography>
      </Box>
      
      <Box sx={{ mb: 3 }}>
        <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
          LLM providers catalogs
        </Typography>
        <CustomSelectMany
          value={selectedCatalogs}
          onChange={setSelectedCatalogs}
          options={catalogs}
          disabled={saving}
        />
      </Box>
      
      <Box sx={{ mb: 3 }}>
        <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
          Data sources catalogs
        </Typography>
        <CustomSelectMany
          value={selectedDataCatalogs}
          onChange={setSelectedDataCatalogs}
          options={dataCatalogs}
          disabled={saving}
        />
      </Box>
      
      <Box sx={{ mb: 2 }}>
        <Typography variant="headingSmall" color="text.primary" sx={{ mb: 1 }}>
          Tools catalogs
        </Typography>
        <CustomSelectMany
          value={selectedToolCatalogs}
          onChange={setSelectedToolCatalogs}
          options={toolCatalogs}
          disabled={saving}
        />
      </Box>
    </ActionModal>
  );
};

export default ManageGroupCatalogsModal;