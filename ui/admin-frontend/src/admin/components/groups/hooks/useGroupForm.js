import { useState, useCallback, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";
import { handleApiError } from "../../../services/utils/errorHandler";
import { CACHE_KEYS } from "../../../utils/constants";

export const useGroupForm = (id, showSnackbar, initialCatalogs = [], initialDataCatalogs = [], initialToolCatalogs = []) => {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);
  
  const navigate = useNavigate();

  const fetchGroup = useCallback(async () => {
    if (!id) return;

    try {
      setLoading(true);
      const response = await teamsService.getTeam(id);
      setName(response.data.attributes.name);
      
      const { catalogues, data_catalogues, tool_catalogues } = response.data.attributes;
      
      if (catalogues) {
        setSelectedCatalogs(catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (data_catalogues) {
        setSelectedDataCatalogs(data_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (tool_catalogues) {
        setSelectedToolCatalogs(tool_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      setLoading(false);
    } catch (error) {
      console.error("Error fetching group", error);
      const apiError = handleApiError(error);
      showSnackbar(apiError.message, "error");
      setLoading(false);
    }
  }, [id, showSnackbar]);

  useEffect(() => {
    if (id) {
      fetchGroup();
    }
  }, [id, fetchGroup]);


  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();

    const groupData = {
      data: {
        type: "Group",
        attributes: {
          name,
          members: selectedUsers.map(user => parseInt(user.id, 10)),
          catalogues: selectedCatalogs.map(cat => parseInt(cat.value, 10)),
          data_catalogues: selectedDataCatalogs.map(cat => parseInt(cat.value, 10)),
          tool_catalogues: selectedToolCatalogs.map(cat => parseInt(cat.value, 10))
        },
      },
    };

    try {
      if (id) {
        await teamsService.updateTeam(id, groupData);

        localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
          operation: "update",
          message: "Team updated successfully",
          timestamp: Date.now()
        }));
      } else {
        await teamsService.createTeam(groupData);

        localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
          operation: "create",
          message: "Team created successfully",
          timestamp: Date.now()
        }));
      }
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error saving group", error);
      const apiError = handleApiError(error);
      showSnackbar(apiError.message, "error");
    }
  }, [id, name, selectedUsers, selectedCatalogs, selectedDataCatalogs, selectedToolCatalogs, navigate, showSnackbar]);

  const handleDeleteClick = useCallback(() => {
    setWarningDialogOpen(true);
  }, []);

  const handleCancelDelete = useCallback(() => {
    setWarningDialogOpen(false);
  }, []);

  const handleConfirmDelete = useCallback(async () => {
    try {
      await teamsService.deleteTeam(id);
      localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
        operation: "delete",
        message: "Team deleted successfully",
        timestamp: Date.now()
      }));
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error deleting team:", error);
      const apiError = handleApiError(error);
      showSnackbar(apiError.message, "error");
    } finally {
      setWarningDialogOpen(false);
    }
  }, [id, navigate, showSnackbar]);

  return useMemo(() => ({
    name,
    setName,
    loading,
    selectedUsers,
    setSelectedUsers,
    selectedCatalogs,
    setSelectedCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    handleSubmit,
    warningDialogOpen,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  }), [
    name,
    loading,
    selectedUsers,
    selectedCatalogs,
    selectedDataCatalogs,
    selectedToolCatalogs,
    warningDialogOpen,
    handleSubmit,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  ]);
};