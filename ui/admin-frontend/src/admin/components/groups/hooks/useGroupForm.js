import { useState, useCallback, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";
import { CACHE_KEYS } from "../../../utils/constants";

export const useGroupForm = (id, initialSelectedUsers = [], initialCatalogs = [], initialDataCatalogs = [], initialToolCatalogs = []) => {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [selectedUsers, setSelectedUsers] = useState(initialSelectedUsers);
  
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
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
      setError("Failed to fetch group");
      setSnackbar({
        open: true,
        message: "Failed to fetch team details",
        severity: "error",
      });
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    if (id) {
      fetchGroup();
    }
  }, [id, fetchGroup]);

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

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
      setError("Failed to save group");
      setSnackbar({
        open: true,
        message: "Failed to save team. Please try again.",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteClick = () => {
    setWarningDialogOpen(true);
  };

  const handleCancelDelete = () => {
    setWarningDialogOpen(false);
  };

  const handleConfirmDelete = async () => {
    try {
      setLoading(true);
      await teamsService.deleteTeam(id);
      localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify({
        operation: "delete",
        message: "Team deleted successfully",
        timestamp: Date.now()
      }));
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error deleting team:", error);
      setSnackbar({
        open: true,
        message: "Failed to delete team. Please try again.",
        severity: "error",
      });
    } finally {
      setWarningDialogOpen(false);
      setLoading(false);
    }
  };

  return {
    name,
    setName,
    loading,
    error,
    selectedUsers,
    setSelectedUsers,
    selectedCatalogs,
    setSelectedCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    handleSubmit,
    snackbar,
    handleCloseSnackbar,
    warningDialogOpen,
    handleDeleteClick,
    handleCancelDelete,
    handleConfirmDelete
  };
};